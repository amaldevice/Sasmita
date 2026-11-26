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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters/openai"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

// orderCheckingProvider asserts the TAD §20 rule at the seam: the run record
// and the user message must exist before the first remote request.
type orderCheckingProvider struct {
	inner model.ModelProvider
	store *repositories.Store
	t     *testing.T
}

func (p *orderCheckingProvider) Name() string { return p.inner.Name() }

func (p *orderCheckingProvider) Stream(ctx context.Context, req model.StreamRequest, emit func(model.StreamEvent)) error {
	msgs, err := p.store.ListMessages(ctx, req.ConversationID, 100)
	if err != nil {
		return err
	}
	if len(msgs) == 0 || msgs[len(msgs)-1].Role != domain.RoleUser {
		p.t.Errorf("user message was not persisted before the provider request")
	}
	runs, err := p.store.ListRuns(ctx, req.ConversationID)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		p.t.Errorf("run record was not persisted before the provider request")
	}
	return p.inner.Stream(ctx, req, emit)
}

func writeSSE(w http.ResponseWriter, answer string) {
	w.Header().Set("Content-Type", "text/event-stream")
	// Four small provider deltas; the app must coalesce them for the UI.
	step := (len(answer) + 3) / 4
	for i := 0; i < len(answer); i += step {
		end := i + step
		if end > len(answer) {
			end = len(answer)
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", answer[i:end])
	}
	fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":5,\"prompt_tokens_details\":{\"cached_tokens\":3}}}\n\n")
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func drain(t *testing.T, h *appsvc.RunHandle) []domain.RunEvent {
	t.Helper()
	var out []domain.RunEvent
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-h.Events():
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatal("timed out waiting for terminal RunEvent")
		}
	}
}

func coalescedText(events []domain.RunEvent) string {
	var b strings.Builder
	for _, ev := range events {
		if ev.Type != domain.EventModelTextDelta {
			continue
		}
		var p domain.TextDeltaPayload
		if json.Unmarshal(ev.Payload, &p) == nil {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func newIDGenerator() func() string {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("id-%d", n)
	}
}

// Seam 1: drive the typed application-service commands against a fake
// OpenAI-compatible provider that plays scripted streams and records the exact
// request bytes. Assert the stored conversation and the golden-prefix property.
func TestSeam1StreamingChatGoldenPrefixAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sasmita.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	now := time.Now()
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work", RootURI: "file:///work", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	answers := []string{"Hello, world!", "Second answer."}
	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
		turn := len(bodies) - 1
		mu.Unlock()
		writeSSE(w, answers[turn])
	}))
	defer srv.Close()

	adapter := openai.Adapter{BaseURL: srv.URL, ProviderID: "openai"}
	cfg := appsvc.Config{
		Provider:     &orderCheckingProvider{inner: adapter, store: store, t: t},
		DefaultModel: "gpt-test",
		CoalesceSize: 8,
		NewID:        newIDGenerator(),
	}
	svc := appsvc.New(store, cfg)

	conv, err := svc.CreateConversation(ctx, "p1", "First", "chat")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetConversation(ctx, conv.ID)
	if err != nil || stored.Title != "First" || stored.ProjectID != "p1" {
		t.Fatalf("stored conversation = %+v (err %v)", stored, err)
	}

	listed, next, err := svc.ListConversations(ctx, "p1", "", 10)
	if err != nil || len(listed) != 1 || listed[0].ID != conv.ID || next != "" {
		t.Fatalf("ListConversations = %+v, next=%q, err=%v", listed, next, err)
	}

	h1, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: "first question", ModelRef: "gpt-test", ModeID: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	events1 := drain(t, h1)
	if len(events1) < 6 {
		t.Fatalf("too few events: %d (%+v)", len(events1), events1)
	}
	for i, ev := range events1 {
		if ev.Seq != uint64(i+1) {
			t.Fatalf("event %d seq = %d, want %d", i, ev.Seq, i+1)
		}
		if ev.RunID != h1.RunID {
			t.Fatalf("event runId = %q, want %q", ev.RunID, h1.RunID)
		}
	}
	if got := coalescedText(events1); got != answers[0] {
		t.Fatalf("coalesced answer = %q, want %q", got, answers[0])
	}
	seen := map[string]bool{}
	for _, ev := range events1 {
		seen[ev.Type] = true
	}
	for _, typ := range []string{domain.EventRunQueued, domain.EventRunStarted, domain.EventModelRequestStarted, domain.EventModelTextDelta, domain.EventModelUsage, domain.EventRunCompleted} {
		if !seen[typ] {
			t.Errorf("missing event type %q", typ)
		}
	}
	if events1[len(events1)-1].Type != domain.EventRunCompleted {
		t.Fatalf("terminal event is %q, must be run.completed", events1[len(events1)-1].Type)
	}

	// Usage and the assistant message must be durable.
	usage, err := store.ListUsage(ctx, h1.RunID)
	if err != nil || len(usage) != 1 || usage[0].InputTokens != 12 || usage[0].CacheReadTokens != 3 || usage[0].Estimated {
		t.Fatalf("usage records = %+v (err %v)", usage, err)
	}
	msgs, err := store.ListMessages(ctx, conv.ID, 100)
	if err != nil || len(msgs) != 2 || msgs[1].Text() != answers[0] {
		t.Fatalf("messages after turn 1 = %+v (err %v)", msgs, err)
	}
	run, err := store.GetRun(ctx, h1.RunID)
	if err != nil || run.Status != domain.RunCompleted || run.FinishedAt == nil {
		t.Fatalf("run after turn 1 = %+v (err %v)", run, err)
	}

	h2, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: "second question", ModeID: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	events2 := drain(t, h2)
	if events2[len(events2)-1].Type != domain.EventRunCompleted {
		t.Fatalf("turn 2 terminal event = %q", events2[len(events2)-1].Type)
	}
	if got := coalescedText(events2); got != answers[1] {
		t.Fatalf("turn 2 answer = %q, want %q", got, answers[1])
	}

	mu.Lock()
	reqs := append([][]byte{}, bodies...)
	mu.Unlock()
	if len(reqs) != 2 {
		t.Fatalf("provider received %d requests, want 2", len(reqs))
	}
	// Golden prefix: turn N request bytes are a byte-exact prefix of turn N+1.
	stable := func(b []byte) []byte { return b[:len(b)-2] } // strip trailing `]}`
	if !bytes.HasPrefix(stable(reqs[1]), stable(reqs[0])) {
		t.Fatalf("golden prefix broken\nturn 1: %s\nturn 2: %s", reqs[0], reqs[1])
	}
	if !bytes.Contains(reqs[1], []byte(answers[0])) || !bytes.Contains(reqs[1], []byte("second question")) {
		t.Fatalf("turn 2 request did not carry prior answer and new question: %s", reqs[1])
	}

	// Simulate a restart: reopen the database and use a fresh service.
	db2, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	svc2 := appsvc.New(repositories.New(db2), appsvc.Config{
		Provider:     adapter,
		DefaultModel: "gpt-test",
		CoalesceSize: 8,
		NewID:        newIDGenerator(),
	})
	detail, err := svc2.GetConversation(ctx, conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, m := range detail.Messages {
		texts = append(texts, m.Text())
	}
	wantTexts := []string{"first question", answers[0], "second question", answers[1]}
	if fmt.Sprint(texts) != fmt.Sprint(wantTexts) {
		t.Fatalf("history after restart = %v, want %v", texts, wantTexts)
	}

	snapshot, err := svc2.GetRunSnapshot(ctx, h2.RunID)
	if err != nil || snapshot == nil || len(snapshot.Events) == 0 {
		t.Fatalf("run snapshot = %+v (err %v)", snapshot, err)
	}
	if snapshot.Events[len(snapshot.Events)-1].Type != domain.EventRunCompleted {
		t.Fatalf("snapshot lost the terminal event: %+v", snapshot.Events)
	}
}
