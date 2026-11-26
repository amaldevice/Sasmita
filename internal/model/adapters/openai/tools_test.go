package openai_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters/openai"
)

func TestEncodeRequestSerializesFrozenToolsBeforeMessages(t *testing.T) {
	req := model.StreamRequest{
		Model:          "gpt-test",
		PromptCacheKey: "session",
		Tools: []model.ToolDefinition{{
			Name: "calculator", Description: "Bounded arithmetic",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string"}}}`),
		}},
		Messages: []model.Message{{Role: "user", Content: "Hi"}},
	}
	got, err := openai.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"gpt-test","stream":true,"stream_options":{"include_usage":true},"prompt_cache_key":"session","tools":[{"type":"function","function":{"description":"Bounded arithmetic","name":"calculator","parameters":{"properties":{"expression":{"type":"string"}},"type":"object"}}}],"messages":[{"role":"user","content":"Hi"}]}`
	if string(got) != want {
		t.Fatalf("tools request mismatch\n got: %s\nwant: %s", got, want)
	}
	if !bytes.HasSuffix(got, []byte(`"messages":[{"role":"user","content":"Hi"}]}`)) {
		t.Fatal("messages must remain the final key for append-only requests")
	}
}

func TestStreamAccumulatesToolCallFragments(t *testing.T) {
	lines := []string{
		`data: {"choices":[{"delta":{"content":"","tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"calculator","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"express"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ion\":\"(2+3)*4\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":9,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":3}}}`,
		`data: [DONE]`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, line := range lines {
			fmt.Fprint(w, line+"\n\n")
		}
	}))
	defer srv.Close()

	adapter := openai.Adapter{BaseURL: srv.URL, ProviderID: "openai"}
	var calls []model.ToolCall
	var blocks []json.RawMessage
	var usage *model.Usage
	if err := adapter.Stream(t.Context(), model.StreamRequest{Model: "m", Messages: []model.Message{{Role: "user", Content: "hi"}}},
		func(ev model.StreamEvent) {
			switch ev.Kind {
			case model.EventToolUse:
				calls = append(calls, *ev.ToolCall)
			case model.EventContentBlocks:
				blocks = append(blocks, ev.ContentBlocks...)
			case model.EventUsage:
				usage = ev.Usage
			}
		}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].ID != "call-1" || calls[0].Name != "calculator" {
		t.Fatalf("tool calls = %+v", calls)
	}
	if string(calls[0].Input) != `{"expression":"(2+3)*4"}` {
		t.Fatalf("tool arguments = %s", calls[0].Input)
	}
	if len(blocks) != 1 || !strings.Contains(string(blocks[0]), `"type":"tool_use"`) ||
		!strings.Contains(string(blocks[0]), `"id":"call-1"`) {
		t.Fatalf("canonical tool blocks = %s", blocks)
	}
	if usage == nil || usage.CacheReadTokens != 3 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestToolCallHistoryReplaysAsOpenAIWireFields(t *testing.T) {
	req := model.StreamRequest{
		Model: "m", PromptCacheKey: "session",
		Messages: []model.Message{
			{Role: "user", Content: "q1"},
			{Role: "assistant", Blocks: []json.RawMessage{
				json.RawMessage(`{"type":"tool_use","id":"call-1","name":"calculator","input":{"expression":"1+1"}}`),
			}},
			{Role: "user", Blocks: []json.RawMessage{
				json.RawMessage(`{"type":"tool_result","tool_use_id":"call-1","content":"2"}`),
			}},
		},
	}
	got, err := openai.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"m","stream":true,"stream_options":{"include_usage":true},"prompt_cache_key":"session","messages":[{"role":"user","content":"q1"},{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"arguments":"{\"expression\":\"1+1\"}","name":"calculator"}}]},{"role":"tool","content":"2","tool_call_id":"call-1"}]}`
	if string(got) != want {
		t.Fatalf("tool history wire mismatch\n got: %s\nwant: %s", got, want)
	}
	canonical, err := openai.CanonicalRequest(req)
	if err != nil || !bytes.Equal(got, canonical) {
		t.Fatalf("canonical bytes differ: %s (err %v)", canonical, err)
	}
}
