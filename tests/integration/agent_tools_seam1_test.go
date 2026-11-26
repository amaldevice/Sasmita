package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// toolSSE plays one Anthropic-compatible turn with an assistant text block and
// one tool_use block. The fake provider records the exact request bytes.
func toolSSE(text, callID, name, input string) string {
	return fmt.Sprintf(strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":9,"output_tokens":0},"content":[]}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%q}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":%q,"name":%q,"input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":%q}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":1}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","usage":{"output_tokens":5}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")+"\n", text, callID, name, input)
}

func newToolService(t *testing.T, srvURL, artifactsDir string, roots []string) (*appsvc.Service, *repositories.Store, string) {
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
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work", RootURI: "file:///work", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc := appsvc.New(store, appsvc.Config{
		Provider:       anthropic.Adapter{BaseURL: srvURL, ProviderID: "anthropic"},
		DefaultModel:   "gpt-test",
		CoalesceSize:   8,
		NewID:          newIDGenerator(),
		EnableTools:    true,
		ArtifactsDir:   artifactsDir,
		WorkspaceRoots: roots,
		ToolOutputCap:  512,
	})
	conv, err := svc.CreateConversation(ctx, "p1", "Tools", "chat")
	if err != nil {
		t.Fatal(err)
	}
	return svc, store, conv.ID
}

func payloadOf(t *testing.T, ev domain.RunEvent) map[string]any {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		t.Fatalf("event %s payload: %v", ev.Type, err)
	}
	return p
}

// Seam 1: a scripted tool call from the fake provider must produce proposed
// and executed events, append its call/result pair to canonical history, and
// leave the request prefix byte-identical across turns.
func TestSeam1ScriptedToolCallKeepsGoldenPrefix(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 0)
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			body = append(body, buf[:n]...)
			if err != nil {
				break
			}
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			fmt.Fprint(w, toolSSE("Let me compute. ", "tu-1", "calculator", `{"expression":"(2+3)*4"}`))
			return
		}
		fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"The answer is 20."}]`,
			`{"input_tokens":11,"output_tokens":3,"cache_creation_input_tokens":0,"cache_read_input_tokens":2}`, "The answer is 20."))
	}))
	defer srv.Close()

	svc, store, convID := newToolService(t, srv.URL, t.TempDir(), nil)
	h, err := svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: convID, Text: "compute for me", ModelRef: "gpt-test", ModeID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, h)
	types := map[string]domain.RunEvent{}
	for _, ev := range events {
		types[ev.Type] = ev
	}
	for _, typ := range []string{
		domain.EventToolProposed, domain.EventToolExecuted, domain.EventModelTextDelta, domain.EventRunCompleted,
	} {
		if _, ok := types[typ]; !ok {
			t.Errorf("missing event %q in %+v", typ, events)
		}
	}
	proposed := payloadOf(t, types[domain.EventToolProposed])
	if proposed["name"] != "calculator" || proposed["executed"] != false {
		t.Fatalf("proposed payload = %+v", proposed)
	}
	executed := payloadOf(t, types[domain.EventToolExecuted])
	if executed["executed"] != true || !strings.Contains(fmt.Sprint(executed["output"]), "20") {
		t.Fatalf("executed payload = %+v", executed)
	}
	if got := coalescedText(events); !strings.Contains(got, "The answer is 20.") {
		t.Fatalf("answer = %q", got)
	}
	if events[len(events)-1].Type != domain.EventRunCompleted {
		t.Fatalf("terminal event = %q", events[len(events)-1].Type)
	}
	for i, ev := range events {
		if ev.Seq != uint64(i+1) {
			t.Fatalf("event %d seq = %d, want %d", i, ev.Seq, i+1)
		}
	}

	if len(bodies) != 2 {
		t.Fatalf("provider received %d requests, want 2", len(bodies))
	}
	if !strings.Contains(string(bodies[0]), `"name":"calculator"`) {
		t.Fatalf("turn 1 has no calculator schema: %s", bodies[0])
	}
	c1 := stripAnthropicBreakpoints(bodies[0])
	c2 := stripAnthropicBreakpoints(bodies[1])
	stable := func(b []byte) []byte { return b[:len(b)-2] }
	if !strings.HasPrefix(string(c2), string(stable(c1))) {
		t.Fatalf("golden prefix broken\nturn 1: %s\nturn 2: %s", c1, c2)
	}
	if !strings.Contains(string(bodies[1]), `"type":"tool_use"`) || !strings.Contains(string(bodies[1]), `"type":"tool_result"`) {
		t.Fatalf("turn 2 did not replay tool call/result: %s", bodies[1])
	}

	msgs, err := store.ListMessages(ctx, convID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 {
		t.Fatalf("canonical messages = %d, want 4: %+v", len(msgs), msgs)
	}
	if msgs[1].Content[0].Type != "tool_use" || msgs[2].Content[0].Type != "tool_result" {
		t.Fatalf("tool pair was not appended canonically: %+v / %+v", msgs[1], msgs[2])
	}
	snapshot, err := svc.GetRunSnapshot(ctx, h.RunID)
	if err != nil || len(snapshot.Events) != len(events) {
		t.Fatalf("snapshot events = %d, live = %d (err %v)", len(snapshot.Events), len(events), err)
	}
}

// Seam 1: output above the tool cap is stored as an artifact and only a short
// summary plus a stable reference enters canonical history and the chat stream.
func TestSeam1OverCapToolResultBecomesArtifact(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	big := strings.Repeat("abcdefghij", 200)
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	artifactsDir := t.TempDir()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			fmt.Fprint(w, toolSSE("Reading. ", "tu-big", "read_project_file", `{"path":"big.txt"}`))
			return
		}
		fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"Done."}]`,
			`{"input_tokens":10,"output_tokens":2,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}`, "Done."))
	}))
	defer srv.Close()

	svc, store, convID := newToolService(t, srv.URL, artifactsDir, []string{root})
	h, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: convID, Text: "read it", ModelRef: "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, h)
	var artifactEv *domain.RunEvent
	for i := range events {
		if events[i].Type == domain.EventToolArtifact {
			artifactEv = &events[i]
		}
	}
	if artifactEv == nil {
		t.Fatalf("no tool.artifact event: %+v", events)
	}
	payload := payloadOf(t, *artifactEv)
	ref, _ := payload["artifactRef"].(string)
	if !strings.HasPrefix(ref, "artifact://") {
		t.Fatalf("artifactRef = %q", ref)
	}
	summary, _ := payload["summary"].(string)
	if len(summary) >= len(big)/2 {
		t.Fatalf("summary is not short: %d bytes", len(summary))
	}
	id := strings.TrimPrefix(ref, "artifact://")
	stored, err := os.ReadFile(filepath.Join(artifactsDir, id+".txt"))
	if err != nil || string(stored) != big {
		t.Fatalf("artifact content mismatch (err %v), got %d bytes", err, len(stored))
	}
	msgs, err := store.ListMessages(ctx, convID, 0)
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := json.Marshal(msgs)
	if strings.Contains(string(canonical), big) {
		t.Fatal("over-cap output leaked into canonical history")
	}
	if !strings.Contains(string(canonical), ref) {
		t.Fatalf("canonical history has no stable artifact reference: %s", canonical)
	}
}
