package integration_test

import (
	"bytes"
	"context"
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
	"github.com/amaldevice/sasmita/internal/modes"
	ctxlayout "github.com/amaldevice/sasmita/internal/context"
	"github.com/amaldevice/sasmita/internal/model/adapters/openai"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

// Seam 1: the Mode instructions reach the model in the active context. For a
// concept or process question the model must call render_ui or create_artifact
// first and write the explanation after the tool result; simple answers stay
// text (ticket #30). The assertion reads the exact wire bytes the provider got.
func TestSeam1ModeInstructionsAreVisualFirst(t *testing.T) {
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
		writeSSE(w, "A visual answer.")
	}))
	defer srv.Close()

	svc := appsvc.New(store, appsvc.Config{
		Provider:     openai.Adapter{BaseURL: srv.URL, ProviderID: "openai"},
		DefaultModel: "gpt-test", NewID: newIDGenerator(),
	})
	conv, err := svc.CreateConversation(ctx, "p1", "Visual first", "chat")
	if err != nil {
		t.Fatal(err)
	}
	h, err := svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: conv.ID, Text: "How does a transformer work?",
		ModelRef: "gpt-test", ModeID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, h)

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("provider received %d requests, want 1", len(bodies))
	}
	for _, phrase := range []string{
		"render_ui", "create_artifact", "FIRST", "after the tool result", "Simple answers stay text",
	} {
		if !bytes.Contains(bodies[0], []byte(phrase)) {
			t.Errorf("Mode instructions missing %q from the wire request: %s", phrase, bodies[0])
		}
	}
	// The instruction is shared product text owned by modes and carried by
	// the single static system message, so it applies to every Mode without
	// adding a second system block or changing the cached prefix shape.
	if !bytes.Contains(bodies[0], []byte(modes.VisualFirstInstructions)) {
		t.Fatalf("shared visual-first instruction missing from the wire request: %s", bodies[0])
	}
	if !bytes.Contains(bodies[0], []byte(ctxlayout.StaticSystemPrompt)) {
		t.Fatalf("static system instruction missing from the wire request: %s", bodies[0])
	}
	if !strings.Contains(modes.VisualFirstInstructions, "render_ui") ||
		!strings.Contains(modes.VisualFirstInstructions, "create_artifact") {
		t.Fatal("modes visual-first instruction does not name both visual tools")
	}
}
