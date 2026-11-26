package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	ctxlayout "github.com/amaldevice/sasmita/internal/context"
	"github.com/amaldevice/sasmita/internal/desktop"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model/adapters/anthropic"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

func newCompactionService(t *testing.T, providerURL string, store *repositories.Store, notes func() ctxlayout.NoteInput) *appsvc.Service {
	t.Helper()
	return appsvc.New(store, appsvc.Config{
		Provider:          anthropic.Adapter{BaseURL: providerURL, ProviderID: "anthropic"},
		DefaultModel:      "gpt-test",
		CoalesceSize:      8,
		NewID:             newIDGenerator(),
		EnableTools:       true,
		ToolOutputCap:     512,
		InputBudgetTokens: 340,
		NoteSource:        notes,
	})
}

func canonicalBodies(bodies [][]byte) [][]byte {
	out := make([][]byte, 0, len(bodies))
	for _, body := range bodies {
		out = append(out, stripAnthropicBreakpoints(body))
	}
	return out
}

func isSummaryBody(body []byte) bool {
	return bytes.Contains(body, []byte("compact-summary-request"))
}

// Seam 1: auto compaction fires near the budget, records what was kept and
// omitted without touching canonical history, costs one cache miss, and the
// next request extends the compacted prefix again (ADR-0001, TAD §11.5).
func TestSeam1AutoCompactionShapeAndAppendOnlyAgain(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "sasmita.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	now := time.Now()
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var bodies [][]byte
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
		n := int(calls.Add(1))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if isSummaryBody(body) {
			summary := `{"goal":["Deliver v1"],"plan":["Verify append-only prefix"]}`
			fmt.Fprint(w, anthropicStreamWithBlocks(`[]`,
				`{"input_tokens":12,"output_tokens":2,"cache_read_input_tokens":11,"cache_creation_input_tokens":0}`, summary))
			return
		}
		switch n {
		case 3:
			fmt.Fprint(w, toolSSE("Computing. ", "tu-1", "calculator", `{"expression":"(2+3)*4"}`))
		default:
			fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"short answer"}]`,
				`{"input_tokens":20,"output_tokens":3,"cache_read_input_tokens":15,"cache_creation_input_tokens":0}`, "short answer"))
		}
	}))
	defer srv.Close()

	noteN := 0
	svc := newCompactionService(t, srv.URL, store, func() ctxlayout.NoteInput {
		noteN++
		return ctxlayout.NoteInput{Attachments: []string{fmt.Sprintf("file:///turn-%d", noteN)}}
	})
	conv, err := svc.CreateConversation(ctx, "p1", "Compaction", "chat")
	if err != nil {
		t.Fatal(err)
	}
	longQuestion := strings.Repeat("question one ", 16)
	questions := []string{longQuestion, strings.Repeat("question two ", 16), strings.Repeat("question three ", 16), "fourth question?", "fifth question?"}
	var handles []*appsvc.RunHandle
	var beforeCompaction []domain.Message
	for i, question := range questions {
		h, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: question, ModelRef: "gpt-test", ModeID: "chat"})
		if err != nil {
			t.Fatal(err)
		}
		events := drain(t, h)
		if events[len(events)-1].Type != domain.EventRunCompleted {
			t.Fatalf("run %d terminal event = %q", i+1, events[len(events)-1].Type)
		}
		handles = append(handles, h)
		if i == 2 {
			beforeCompaction, err = store.ListMessages(ctx, conv.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// Canonical history was never rewritten: the pre-compaction messages are
	// still present byte-for-byte, with only later messages appended.
	after, err := store.ListMessages(ctx, conv.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := json.Marshal(beforeCompaction)
	b2, _ := json.Marshal(after)
	if len(after) < len(beforeCompaction) || !reflect.DeepEqual(after[:len(beforeCompaction)], beforeCompaction) {
		t.Fatalf("canonical history changed\nbefore: %s\nafter: %s", b1, b2)
	}
	notes := 0
	for _, m := range after {
		for _, part := range m.Content {
			if part.Type == "context_note" {
				notes++
				if m.Role != domain.RoleUser {
					t.Fatalf("context note on %s turn", m.Role)
				}
			}
		}
	}
	if notes != len(questions) {
		t.Fatalf("context notes = %d, want %d", notes, len(questions))
	}

	// One compaction was recorded, with kept and omitted message IDs.
	events, err := store.ListRunEvents(ctx, handles[2].RunID)
	if err != nil {
		t.Fatal(err)
	}
	var record *domain.Compaction
	for _, ev := range events {
		if ev.Type != domain.EventContextCompacted {
			continue
		}
		var payload domain.CompactionPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		record = &payload.Compaction
	}
	if record == nil {
		t.Fatalf("no context.compacted event: %+v", events)
	}
	if record.ModelRef != "gpt-test" || record.ID == "" || record.CreatedAt.IsZero() ||
		len(record.KeptMessageIDs) == 0 || len(record.OmittedMessageIDs) == 0 {
		t.Fatalf("compaction record = %+v", record)
	}
	if !strings.Contains(record.Summary, "Deliver v1") {
		t.Fatalf("structured summary = %q", record.Summary)
	}
	if fmt.Sprint(record.KeptMessageIDs) != fmt.Sprint(idsOf(beforeCompaction[len(beforeCompaction)-4:])) {
		t.Fatalf("kept = %v, want last four messages", record.KeptMessageIDs)
	}

	mu.Lock()
	reqs := append([][]byte(nil), bodies...)
	mu.Unlock()
	canonical := canonicalBodies(reqs)
	var summaryIndex int
	for i, body := range reqs {
		if isSummaryBody(body) {
			summaryIndex = i
		}
	}
	count := 0
	for _, body := range reqs {
		if isSummaryBody(body) {
			count++
		}
	}
	if count != 1 || summaryIndex != 4 {
		t.Fatalf("summary requests = %d, first at %d (total %d): %s", count, summaryIndex, len(reqs), summarizeBodies(reqs))
	}
	stable := func(b []byte) []byte { return b[:len(b)-2] }
	// The summary request extends the last request sent before compaction.
	if !bytes.HasPrefix(stable(canonical[summaryIndex]), stable(canonical[summaryIndex-1])) {
		t.Fatalf("summary request left the cached prefix\nprev: %s\nsummary: %s", reqs[summaryIndex-1], reqs[summaryIndex])
	}
	if !bytes.Contains(reqs[summaryIndex], []byte(`"model":"gpt-test"`)) {
		t.Fatalf("summary request used a different model: %s", reqs[summaryIndex])
	}
	// The compacted request is the one cache miss: frozen tools, system
	// instruction, structured summary, then the last two user turns with the
	// complete tool-call pair.
	compacted := reqs[summaryIndex+1]
	for _, want := range []string{`"name":"calculator"`, "compaction summary", "Deliver v1", `"type":"tool_use"`, `"type":"tool_result"`, "question three", "fourth question?"} {
		if !bytes.Contains(compacted, []byte(want)) {
			t.Fatalf("compacted request missing %q: %s", want, compacted)
		}
	}
	if bytes.HasPrefix(stable(canonical[summaryIndex+1]), stable(canonical[summaryIndex])) {
		t.Fatal("compacted prefix still extends the pre-compaction request; no cache miss")
	}
	// After that miss, the next turn extends the compacted prefix again.
	if !bytes.HasPrefix(stable(canonical[summaryIndex+2]), stable(canonical[summaryIndex+1])) {
		t.Fatalf("golden prefix not restored after compaction\ncompacted: %s\nnext: %s", reqs[summaryIndex+1], reqs[summaryIndex+2])
	}
}

// The user-requested compaction command is a typed desktop binding. It records
// the new prefix durably and leaves canonical history untouched.
func TestSeam1ManualCompactionCommandRecordsPrefix(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "sasmita.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	now := time.Now()
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if isSummaryBody(body) {
			summary := `{"goal":["Manual summary"],"facts":["kept"]}`
			fmt.Fprint(w, anthropicStreamWithBlocks(`[]`,
				`{"input_tokens":9,"output_tokens":2,"cache_read_input_tokens":8,"cache_creation_input_tokens":0}`, summary))
			return
		}
		fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"answer"}]`,
			`{"input_tokens":20,"output_tokens":3,"cache_read_input_tokens":15,"cache_creation_input_tokens":0}`, "answer"))
	}))
	defer srv.Close()

	svc := appsvc.New(store, appsvc.Config{
		Provider:     anthropic.Adapter{BaseURL: srv.URL, ProviderID: "anthropic"},
		DefaultModel: "gpt-test", CoalesceSize: 8, NewID: newIDGenerator(),
	})
	conv, err := svc.CreateConversation(ctx, "p1", "Manual", "chat")
	if err != nil {
		t.Fatal(err)
	}
	h, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: strings.Repeat("long question ", 20), ModelRef: "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, h)
	// A second turn gives compaction a kept tail (last two user turns).
	hFirst, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: "second question", ModelRef: "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, hFirst)
	hSecond, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: "third question", ModelRef: "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, hSecond)
	before, err := store.ListMessages(ctx, conv.ID, 0)
	if err != nil {
		t.Fatal(err)
	}

	var bindings desktop.Bindings = desktop.NewService(svc)
	dto, err := bindings.CompactConversation(ctx, conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dto.ID == "" || !strings.Contains(dto.Summary, "Manual summary") || len(dto.KeptMessageIDs) == 0 {
		t.Fatalf("compaction DTO = %+v", dto)
	}
	if dto.ModelRef != "gpt-test" || dto.RunID != hSecond.RunID {
		t.Fatalf("compaction DTO model=%q run=%q", dto.ModelRef, dto.RunID)
	}
	after, err := store.ListMessages(ctx, conv.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("manual compaction rewrote canonical history")
	}
	events, err := store.ListRunEvents(ctx, hSecond.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, ev := range events {
		if ev.Type == domain.EventContextCompacted {
			found = true
		}
	}
	if !found {
		t.Fatalf("no context.compacted event: %+v", events)
	}

	// The next run uses the compacted prefix (one cache miss).
	h2, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: "next question", ModelRef: "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, h2)
	mu.Lock()
	reqs := append([][]byte(nil), bodies...)
	mu.Unlock()
	if len(reqs) != 5 || !isSummaryBody(reqs[3]) {
		t.Fatalf("requests = %s", summarizeBodies(reqs))
	}
	if !bytes.Contains(reqs[4], []byte("Manual summary")) {
		t.Fatalf("next run did not use the summary: %s", reqs[4])
	}
}

func idsOf(messages []domain.Message) []string {
	out := make([]string, 0, len(messages))
	for _, m := range messages {
		out = append(out, m.ID)
	}
	return out
}

func summarizeBodies(bodies [][]byte) string {
	var out []string
	for i, body := range bodies {
		out = append(out, fmt.Sprintf("%d:%d", i, len(body)))
	}
	return strings.Join(out, " ")
}
