package integration_test

import (
	"bytes"
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
	"github.com/amaldevice/sasmita/internal/model/adapters/openai"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

func openaiToolSSE(text, callID, name, input string) string {
	lines := []string{
		fmt.Sprintf(`data: {"choices":[{"delta":{"content":%q}}]}`, text),
		fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":%q,"type":"function","function":{"name":%q,"arguments":""}}]}}]}`, callID, name),
		fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":%q}}]}}]}`, input),
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: {"choices":[{"delta":{"content":"Selesai."}}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":2}}}`,
		`data: [DONE]`,
	}
	return strings.Join(lines, "\n\n") + "\n\n"
}

// Seam 1: the OpenAI-compatible adapter is a first-class tool adapter. Its
// scripted tool call drives policy + execution, replays as tool_calls/tool
// messages on the next turn, and keeps the golden prefix intact.
func TestSeam1OpenAIToolCallKeepsGoldenPrefix(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(t.TempDir() + "/sasmita.db")
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

	var calls atomic.Int32
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			fmt.Fprint(w, openaiToolSSE("Menghitung. ", "tu-oai-1", "calculator", `{"expression":"(2+3)*4"}`))
			return
		}
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"Jawaban 20."}}]}`+"\n\n"+
			`data: {"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":2}}}`+"\n\n"+
			"data: [DONE]\n\n")
	}))
	defer srv.Close()

	svc := appsvc.New(store, appsvc.Config{
		Provider:     openai.Adapter{BaseURL: srv.URL, ProviderID: "openai"},
		DefaultModel: "gpt-test", NewID: newIDGenerator(),
		EnableTools: true, ArtifactsDir: t.TempDir(),
	})
	conv, err := svc.CreateConversation(ctx, "p1", "OpenAI tools", "chat")
	if err != nil {
		t.Fatal(err)
	}
	h, err := svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: conv.ID, Text: "hitung", ModelRef: "gpt-test", ModeID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, h)
	types := map[string]bool{}
	for _, ev := range events {
		types[ev.Type] = true
	}
	for _, want := range []string{domain.EventToolProposed, domain.EventToolExecuted, domain.EventRunCompleted} {
		if !types[want] {
			t.Fatalf("missing %q in %+v", want, events)
		}
	}
	if !strings.Contains(coalescedText(events), "Jawaban 20.") {
		t.Fatalf("answer = %q", coalescedText(events))
	}
	if len(bodies) != 2 {
		t.Fatalf("provider received %d requests, want 2", len(bodies))
	}
	if !bytes.Contains(bodies[0], []byte(`"name":"calculator"`)) {
		t.Fatalf("turn 1 has no tool schemas: %s", bodies[0])
	}
	if !bytes.Contains(bodies[1], []byte(`"tool_calls"`)) || !bytes.Contains(bodies[1], []byte(`"tool_call_id":"tu-oai-1"`)) {
		t.Fatalf("turn 2 did not replay the OpenAI tool call: %s", bodies[1])
	}
	stable := func(b []byte) []byte { return b[:len(b)-2] }
	if !bytes.HasPrefix(bodies[1], stable(bodies[0])) {
		t.Fatalf("golden prefix broken across tool calls\nfirst:  %s\nsecond: %s", bodies[0], bodies[1])
	}
	msgs, err := store.ListMessages(ctx, conv.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 || msgs[1].Content[0].Type != "tool_use" || msgs[2].Content[0].Type != "tool_result" {
		t.Fatalf("canonical tool pair missing: %+v", msgs)
	}
	var wire struct {
		Messages []struct {
			Role       string `json:"role"`
			ToolCalls  []struct {
				Function struct{ Arguments string `json:"arguments"` } `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(bodies[1], &wire); err != nil {
		t.Fatal(err)
	}
	var callArgs string
	toolMessage := false
	for _, m := range wire.Messages {
		if m.Role == "tool" {
			toolMessage = true
		}
		if m.Role == "assistant" && len(m.ToolCalls) == 1 {
			callArgs = m.ToolCalls[0].Function.Arguments
		}
	}
	if !toolMessage || callArgs != `{"expression":"(2+3)*4"}` {
		t.Fatalf("wire tool call = %+v", wire.Messages)
	}
}
