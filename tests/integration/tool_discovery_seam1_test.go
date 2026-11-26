package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model/adapters/anthropic"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

// Discovery results carry full schemas, so this service uses the documented
// 8 KB cap instead of the smaller cap used by the #22 tool tests.
func newDiscoveryService(t *testing.T, srvURL string, budget ...int) (*appsvc.Service, string) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "sasmita.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	now := time.Now()
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	cfg := appsvc.Config{
		Provider:      anthropic.Adapter{BaseURL: srvURL, ProviderID: "anthropic"},
		DefaultModel:  "gpt-test",
		CoalesceSize:  8,
		NewID:         newIDGenerator(),
		EnableTools:   true,
		ArtifactsDir:  t.TempDir(),
		ToolOutputCap: 8192,
	}
	if len(budget) > 0 {
		cfg.InputBudgetTokens = budget[0]
	}
	svc := appsvc.New(store, cfg)
	conv, err := svc.CreateConversation(ctx, "p1", "Discovery", "chat")
	if err != nil {
		t.Fatal(err)
	}
	return svc, conv.ID
}

// discoverySSE plays one Anthropic-compatible turn with a single tool call.
// The fake provider records exact request bytes for the golden-prefix check.
func discoverySSE(text, callID, name, input string) string {
	return toolSSE(text, callID, name, input)
}

// Seam 1: find_tools ranks local enabled tools, returns their schemas in a tool
// result bounded to 8 KB, and never changes the frozen tool list mid-run.
func TestSeam1FindToolsRanksAndCapsResult(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		switch calls.Add(1) {
		case 1:
			fmt.Fprint(w, discoverySSE("Mencari. ", "ft-1", "find_tools", `{"query":"load skill chart"}`))
		case 2:
			fmt.Fprint(w, discoverySSE("Memanggil. ", "ft-2", "call_tool", `{"name":"load_skill","args":{"component":"chart"}}`))
		default:
			fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"Selesai."}]`,
				`{"input_tokens":11,"output_tokens":3,"cache_creation_input_tokens":0,"cache_read_input_tokens":2}`, "Selesai."))
		}
	}))
	defer srv.Close()

	svc, convID := newDiscoveryService(t, srv.URL)
	h, err := svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: convID, Text: "cari tool tabel", ModelRef: "gpt-test", ModeID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, h)
	var found, called string
	for _, ev := range events {
		if ev.Type != domain.EventToolExecuted {
			continue
		}
		p := payloadOf(t, ev)
		name, _ := p["name"].(string)
		output, _ := p["output"].(string)
		switch name {
		case "find_tools":
			found = output
		case "load_skill":
			called = output
		}
	}
	if found == "" {
		t.Fatalf("find_tools did not execute: %+v", events)
	}
	if !strings.Contains(found, "load_skill") || !strings.Contains(found, "inputSchema") {
		t.Fatalf("find_tools result did not rank the tool with its schema: %s", found)
	}
	if len(found) > 8192 {
		t.Fatalf("find_tools result is %d bytes, want <= 8192", len(found))
	}
	if !strings.Contains(called, "chart:") {
		t.Fatalf("call_tool did not execute load_skill: %q", called)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 3 {
		t.Fatalf("provider received %d requests, want 3", len(bodies))
	}
	for i, body := range bodies {
		wire := string(body)
		if !strings.Contains(wire, `"name":"find_tools"`) || !strings.Contains(wire, `"name":"call_tool"`) {
			t.Errorf("request %d changed the frozen tool list: %s", i, wire)
		}
	}
	c1, c2 := stripAnthropicBreakpoints(bodies[0]), stripAnthropicBreakpoints(bodies[1])
	stable := func(b []byte) []byte { return b[:len(b)-2] }
	if !strings.HasPrefix(string(c2), string(stable(c1))) {
		t.Fatalf("discovery broke the golden prefix\nfirst: %s\nsecond: %s", c1, c2)
	}
}

// Seam 1: call_tool validates the target schema in Go before execution.
func TestSeam1CallToolValidatesTargetSchema(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			fmt.Fprint(w, discoverySSE("Memeriksa. ", "v-1", "call_tool", `{"name":"calculator","args":{}}`))
			return
		}
		fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"Selesai."}]`,
			`{"input_tokens":10,"output_tokens":2,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}`, "Selesai."))
	}))
	defer srv.Close()

	svc, convID := newDiscoveryService(t, srv.URL)
	h, err := svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: convID, Text: "hitung", ModelRef: "gpt-test", ModeID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, h)
	var failed string
	for _, ev := range events {
		if ev.Type == domain.EventToolFailed {
			p := payloadOf(t, ev)
			failed, _ = p["error"].(string)
			if p["name"] != "calculator" {
				t.Fatalf("failed tool name = %v, want the real tool calculator", p["name"])
			}
		}
		if ev.Type == domain.EventToolExecuted {
			t.Fatalf("invalid call was executed: %+v", ev)
		}
	}
	if !strings.Contains(failed, "expression") {
		t.Fatalf("validation error = %q", failed)
	}
}

// Seam 1: load_skill through call_tool returns the bounded skill content.
func TestSeam1CallToolReturnsSkillContent(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			fmt.Fprint(w, discoverySSE("Membaca. ", "s-1", "call_tool", `{"name":"load_skill","args":{"component":"chart"}}`))
			return
		}
		fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"Selesai."}]`,
			`{"input_tokens":10,"output_tokens":2,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}`, "Selesai."))
	}))
	defer srv.Close()

	svc, convID := newDiscoveryService(t, srv.URL)
	h, err := svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: convID, Text: "aturan chart", ModelRef: "gpt-test", ModeID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, h)
	found := false
	for _, ev := range events {
		if ev.Type != domain.EventToolExecuted {
			continue
		}
		p := payloadOf(t, ev)
		output, _ := p["output"].(string)
		if p["name"] == "load_skill" && strings.Contains(output, "chart:") && strings.Contains(output, "values") {
			found = true
		}
	}
	if !found {
		t.Fatalf("load_skill content missing from tool results: %+v", events)
	}
}

// Seam 1: policy and approval evaluate the real tool behind call_tool, so the
// approval card names write_artifact, never the generic envelope.
func TestSeam1CallToolPolicyUsesRealToolID(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			fmt.Fprint(w, discoverySSE("Menulis. ", "w-1", "call_tool",
				`{"name":"write_artifact","args":{"name":"chart","content":"x","overwrite":true}}`))
			return
		}
		fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"Selesai."}]`,
			`{"input_tokens":10,"output_tokens":2,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}`, "Selesai."))
	}))
	defer srv.Close()

	svc, convID := newDiscoveryService(t, srv.URL)
	h, err := svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: convID, Text: "timpa artifact", ModelRef: "gpt-test", ModeID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	var approvalID, payloadToolID string
	for ev := range h.Events() {
		if ev.Type != domain.EventApprovalRequired {
			continue
		}
		var payload domain.ApprovalPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		approvalID, payloadToolID = payload.ApprovalID, payload.ToolID
		break
	}
	if approvalID == "" || payloadToolID != "write_artifact" {
		t.Fatalf("approval card = %q for tool %q", approvalID, payloadToolID)
	}
	approval, err := svc.GetApproval(ctx, approvalID)
	if err != nil || approval.ToolID != "write_artifact" {
		t.Fatalf("stored approval = %+v (err %v)", approval, err)
	}
	if err := svc.ResolveApproval(ctx, approvalID, domain.ApprovalAllowOnce, nil); err != nil {
		t.Fatal(err)
	}
	events := drain(t, h)
	for _, ev := range events {
		if ev.Type == domain.EventToolExecuted {
			p := payloadOf(t, ev)
			if p["name"] != "write_artifact" {
				t.Fatalf("executed tool = %v, want write_artifact", p["name"])
			}
			return
		}
	}
	t.Fatalf("write_artifact never executed: %+v", events)
}

func nativeToolNames(t *testing.T, body []byte) []string {
	t.Helper()
	var req struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(req.Tools))
	for _, tool := range req.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// Seam 1: v1 has no discovery-only tools, so compaction must not add native
// tools to the frozen request. Meta-tools stay native before and after.
func TestSeam1FrozenToolListUnchangedAtCompaction(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.Contains(string(body), "compact-summary-request") {
			fmt.Fprint(w, anthropicStreamWithBlocks(`[]`,
				`{"input_tokens":12,"output_tokens":2,"cache_read_input_tokens":11,"cache_creation_input_tokens":0}`,
				`{"goal":["Deliver v1"]}`))
			return
		}
		switch calls.Add(1) {
		case 1:
			fmt.Fprint(w, discoverySSE("Membaca. ", "p-1", "call_tool",
				`{"name":"load_skill","args":{"component":"chart"}}`))
		case 2:
			fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"Jawaban."}]`,
				`{"input_tokens":12,"output_tokens":2,"cache_read_input_tokens":11,"cache_creation_input_tokens":0}`, "Jawaban."))
		case 3:
			fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"Kedua."}]`,
				`{"input_tokens":12,"output_tokens":2,"cache_read_input_tokens":11,"cache_creation_input_tokens":0}`, "Kedua."))
		default:
			fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"Lanjutan."}]`,
				`{"input_tokens":12,"output_tokens":2,"cache_read_input_tokens":11,"cache_creation_input_tokens":0}`, "Lanjutan."))
		}
	}))
	defer srv.Close()

	svc, convID := newDiscoveryService(t, srv.URL, 200)
	question := strings.Repeat("explain the table ", 20)
	h, err := svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: convID, Text: question, ModelRef: "gpt-test", ModeID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, h)
	// A second user turn gives compaction both an omitted and a kept tail.
	h2, err := svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: convID, Text: question + " again", ModelRef: "gpt-test", ModeID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, h2)
	compacted := false
	for _, ev := range events {
		if ev.Type == domain.EventContextCompacted {
			compacted = true
		}
	}
	if !compacted {
		t.Fatalf("compaction did not fire: %+v", events)
	}

	h3, err := svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: convID, Text: "next question", ModelRef: "gpt-test", ModeID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, h3)

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) < 3 {
		t.Fatalf("provider received %d requests, want at least 3", len(bodies))
	}
	// Summary requests use the pre-compaction frozen list; compare ordinary
	// conversation requests before and after the prefix reset.
	ordinary := make([][]byte, 0, len(bodies))
	for _, body := range bodies {
		if !strings.Contains(string(body), "compact-summary-request") {
			ordinary = append(ordinary, body)
		}
	}
	before := nativeToolNames(t, ordinary[0])
	last := nativeToolNames(t, ordinary[len(ordinary)-1])
	if len(before) != len(last) {
		t.Fatalf("compaction changed the frozen tool list: %v -> %v", before, last)
	}
	for i, name := range before {
		if name != last[i] {
			t.Fatalf("compaction changed the frozen tool list: %v -> %v", before, last)
		}
	}
	// The meta-tools remain native in both lists.
	for _, body := range [][]byte{bodies[0], bodies[len(bodies)-1]} {
		names := nativeToolNames(t, body)
		found := map[string]bool{}
		for _, name := range names {
			found[name] = true
		}
		if !found["find_tools"] || !found["call_tool"] {
			t.Fatalf("meta-tools missing from frozen list: %v", names)
		}
	}
}
