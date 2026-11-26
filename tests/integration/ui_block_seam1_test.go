package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/genui"
	"github.com/amaldevice/sasmita/internal/model/adapters/anthropic"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

const seamUIBlockSpec = `{
	"schemaVersion":"sasmita.ui/v1","blockId":"uib_seam","kind":"interactive",
	"title":"Rounding explorer","summary":"Compare rounding modes.",
	"state":{"threshold":76},
	"components":[
		{"id":"threshold","type":"slider","label":"Threshold","bind":"state.threshold","min":0,"max":100,"step":1},
		{"id":"series","type":"chart","label":"Error","values":[{"label":"0","value":1}]}
	]
}`

// uiBlockService mirrors newToolService but with the GenUI built-ins the app
// registers into #22's registry when tools are enabled.
func uiBlockService(t *testing.T, srvURL string) (*appsvc.Service, *repositories.Store, string) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(fmt.Sprintf("%s/sasmita.db", t.TempDir()))
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
	svc := appsvc.New(store, appsvc.Config{
		Provider:     anthropic.Adapter{BaseURL: srvURL, ProviderID: "anthropic"},
		DefaultModel: "gpt-test",
		CoalesceSize: 8,
		NewID:        newIDGenerator(),
		EnableTools:  true,
		ArtifactsDir: t.TempDir(),
	})
	conv, err := svc.CreateConversation(ctx, "p1", "UI blocks", "chat")
	if err != nil {
		t.Fatal(err)
	}
	return svc, store, conv.ID
}

func scriptedRenderServer(t *testing.T, spec string) *httptest.Server {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			input, err := json.Marshal(json.RawMessage(spec))
			if err != nil {
				t.Errorf("marshal tool input: %v", err)
			}
			fmt.Fprint(w, toolSSE("Building a visual. ", "tu-ui", "render_ui", string(input)))
			return
		}
		fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"See the chart."}]`,
			`{"input_tokens":12,"output_tokens":4,"cache_creation_input_tokens":0,"cache_read_input_tokens":1}`, "See the chart."))
	}))
	return srv
}

// Seam 1: a scripted render_ui call must emit one persisted ui.block event with
// the validated spec, append it to canonical history, and keep the full spec
// out of the tool result that returns to the model.
func TestSeam1RenderUIPersistsUIBlock(t *testing.T) {
	ctx := context.Background()
	srv := scriptedRenderServer(t, seamUIBlockSpec)
	defer srv.Close()
	svc, store, convID := uiBlockService(t, srv.URL)

	h, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: convID, Text: "show a chart", ModelRef: "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, h)
	var block *domain.RunEvent
	var executed *domain.RunEvent
	for i := range events {
		switch events[i].Type {
		case domain.EventUIBlock:
			block = &events[i]
		case domain.EventToolExecuted:
			executed = &events[i]
		}
	}
	if block == nil {
		t.Fatalf("no ui.block event: %+v", events)
	}
	var payload map[string]any
	if err := json.Unmarshal(block.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	rawSpec, _ := json.Marshal(payload["spec"])
	if !strings.Contains(string(rawSpec), `"blockId":"uib_seam"`) {
		t.Fatalf("ui.block payload = %s", block.Payload)
	}
	if payload["reason"] != nil && payload["reason"] != "" {
		t.Fatalf("valid block has a reason: %v", payload["reason"])
	}
	if executed == nil || strings.Contains(string(executed.Payload), `\"components\"`) {
		t.Fatalf("tool result leaked the full spec: %+v", executed)
	}
	if text := coalescedText(events); !strings.Contains(text, "See the chart.") {
		t.Fatalf("text answer was not preserved: %q", text)
	}

	msgs, err := store.ListMessages(ctx, convID, 0)
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := json.Marshal(msgs)
	if !strings.Contains(string(canonical), `"type":"ui_block"`) || !strings.Contains(string(canonical), "uib_seam") {
		t.Fatalf("UI block not persisted canonically: %s", canonical)
	}

	snapshot, err := svc.GetRunSnapshot(ctx, h.RunID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range snapshot.Events {
		if ev.Type == domain.EventUIBlock {
			found = true
		}
	}
	if !found {
		t.Fatal("ui.block missing from the reload snapshot")
	}
}

// Seam 1: an invalid spec fails closed to a short reason in the ui.block event.
func TestSeam1InvalidUIBlockFailsClosed(t *testing.T) {
	ctx := context.Background()
	srv := scriptedRenderServer(t, `{"schemaVersion":"sasmita.ui/v2","blockId":"bad","summary":"x","components":[{"id":"x","type":"iframe"}]}`)
	defer srv.Close()
	svc, _, convID := uiBlockService(t, srv.URL)
	h, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: convID, Text: "show a chart", ModelRef: "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, h)
	var fallback bool
	for _, ev := range events {
		if ev.Type != domain.EventUIBlock {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		reason, _ := payload["reason"].(string)
		if reason != "" && len(reason) < 120 && payload["spec"] == nil {
			fallback = true
		}
	}
	if !fallback {
		t.Fatalf("invalid spec did not fail closed: %+v", events)
	}
}

// Seam 1: a user-committed state patch is validated against the stored spec and
// persisted as an append-only ui.state event; unknown keys are rejected.
func TestSeam1UIStatePatchPersistsAndValidates(t *testing.T) {
	ctx := context.Background()
	srv := scriptedRenderServer(t, seamUIBlockSpec)
	defer srv.Close()
	svc, _, convID := uiBlockService(t, srv.URL)
	h, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: convID, Text: "show a chart", ModelRef: "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, h)
	_ = events

	if err := svc.SaveUIState(ctx, h.RunID, "uib_seam", json.RawMessage(`{"ghost":1}`)); err == nil {
		t.Fatal("unknown state key accepted")
	}
	if err := svc.SaveUIState(ctx, h.RunID, "uib_seam", json.RawMessage(`{"threshold":80}`)); err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.GetRunSnapshot(ctx, h.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var stored *domain.RunEvent
	for i := range snapshot.Events {
		if snapshot.Events[i].Type == domain.EventUIState {
			stored = &snapshot.Events[i]
		}
	}
	if stored == nil || !strings.Contains(string(stored.Payload), `"threshold":80`) {
		t.Fatalf("ui.state not persisted: %+v", stored)
	}
	if genui.SchemaVersion != "sasmita.ui/v1" {
		t.Fatal("schema version constant drifted")
	}
}
