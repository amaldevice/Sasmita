package anthropic_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters/anthropic"
)

const anthropicCacheKey = "1723376043e8a9d0090025f147440b99" // SHA-256("conv-123"), first 16 bytes

func layoutRequest() model.StreamRequest {
	return model.StreamRequest{
		Model:          "claude-test",
		PromptCacheKey: anthropicCacheKey,
		Tools: []model.ToolDefinition{
			{Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		Messages: []model.Message{
			{Role: "system", Content: "static system"},
			{Role: "user", Content: "q1"},
			{Role: "assistant", Blocks: []json.RawMessage{
				json.RawMessage(`{"signature":"sig-abc","type":"thinking","thinking":"hmm"}`),
				json.RawMessage(`{"type":"text","text":"a1"}`),
			}},
			{Role: "user", Content: "q2"},
		},
	}
}

func stripBreakpoints(b []byte) []byte {
	s := string(b)
	s = strings.ReplaceAll(s, `,"cache_control":{"type":"ephemeral","ttl":"1h"}`, "")
	s = strings.ReplaceAll(s, `,"cache_control":{"type":"ephemeral","ttl":"5m"}`, "")
	return []byte(s)
}

func TestEncodeRequestHasStaticSystemSessionKeyAndTwoBreakpoints(t *testing.T) {
	wire, err := anthropic.EncodeRequest(layoutRequest())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"claude-test","max_tokens":1024,"stream":true,"session_id":"` + anthropicCacheKey + `","tools":[{"name":"lookup","input_schema":{"type":"object"}}],` +
		`"system":[{"type":"text","text":"static system","cache_control":{"type":"ephemeral","ttl":"1h"}}],` +
		`"messages":[{"role":"user","content":[{"type":"text","text":"q1"}]},` +
		`{"role":"assistant","content":[{"signature":"sig-abc","type":"thinking","thinking":"hmm"},{"type":"text","text":"a1"}]},` +
		`{"role":"user","content":[{"type":"text","text":"q2","cache_control":{"type":"ephemeral","ttl":"5m"}}]}]}`
	if string(wire) != want {
		t.Fatalf("request bytes mismatch\n got: %s\nwant: %s", wire, want)
	}

	canonical, err := anthropic.CanonicalRequest(layoutRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stripBreakpoints(wire), canonical) {
		t.Fatalf("breakpoints changed canonical bytes\nwire:      %s\ncanonical: %s", wire, canonical)
	}
	if !bytes.Contains(canonical, []byte(`{"signature":"sig-abc","type":"thinking","thinking":"hmm"}`)) {
		t.Fatalf("raw thinking/signature block was re-serialized: %s", canonical)
	}
}

func TestThirdBreakpointOnlyForLargeTurn(t *testing.T) {
	blocks := make([]json.RawMessage, 0, 25)
	for i := 0; i < 25; i++ {
		blocks = append(blocks, json.RawMessage(fmt.Sprintf(`{"type":"text","text":"b%d"}`, i)))
	}
	req := model.StreamRequest{Model: "m", Messages: []model.Message{
		{Role: "system", Content: "static"},
		{Role: "user", Blocks: blocks},
	}}
	wire, err := anthropic.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(wire), `"cache_control"`); n != 3 {
		t.Fatalf("breakpoint count = %d, want 3: %s", n, wire)
	}

	small := model.StreamRequest{Model: "m", Messages: []model.Message{
		{Role: "system", Content: "static"},
		{Role: "user", Content: "q"},
	}}
	wireSmall, err := anthropic.EncodeRequest(small)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(wireSmall), `"cache_control"`); n != 2 {
		t.Fatalf("small request breakpoint count = %d, want 2: %s", n, wireSmall)
	}
}

func TestCanonicalPrefixExtendsAcrossTurnsWithRawBlocks(t *testing.T) {
	first := []model.Message{
		{Role: "system", Content: "static system"},
		{Role: "user", Content: "q1"},
	}
	second := append(append([]model.Message{}, first...),
		model.Message{Role: "assistant", Blocks: []json.RawMessage{
			json.RawMessage(`{"signature":"sig","type":"thinking","thinking":"hmm"}`),
		}},
		model.Message{Role: "user", Content: "q2"},
	)
	req := func(msgs []model.Message) model.StreamRequest {
		return model.StreamRequest{Model: "m", PromptCacheKey: anthropicCacheKey, Messages: msgs}
	}
	p1, err := anthropic.CanonicalRequest(req(first))
	if err != nil {
		t.Fatal(err)
	}
	p2, err := anthropic.CanonicalRequest(req(second))
	if err != nil {
		t.Fatal(err)
	}
	stable := func(b []byte) []byte { return b[:len(b)-2] }
	if !bytes.HasPrefix(stable(p2), stable(p1)) {
		t.Fatalf("golden prefix broken\nturn 1: %s\nturn 2: %s", p1, p2)
	}
}

func TestStreamCapturesRawProviderThinkingAndSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"type":"message_start","message":{"usage":{"input_tokens":7,"output_tokens":0,"cache_creation_input_tokens":1,"cache_read_input_tokens":2},"content":[{"signature":"sig-raw","type":"thinking","thinking":"hmm"},{"type":"text","text":"answer"}]}}`+"\n\n")
		fmt.Fprint(w, `data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"answer"}}`+"\n\n")
		fmt.Fprint(w, `data: {"type":"message_delta","usage":{"output_tokens":3}}`+"\n\n")
		fmt.Fprint(w, `data: {"type":"message_stop"}`+"\n\n")
	}))
	defer srv.Close()

	var blocks []json.RawMessage
	var usage *model.Usage
	adapter := anthropic.Adapter{BaseURL: srv.URL, ProviderID: "anthropic"}
	if err := adapter.Stream(t.Context(), model.StreamRequest{Model: "m", Messages: []model.Message{{Role: "user", Content: "q"}}},
		func(ev model.StreamEvent) {
			switch ev.Kind {
			case model.EventContentBlocks:
				blocks = ev.ContentBlocks
			case model.EventUsage:
				usage = ev.Usage
			}
		}); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || string(blocks[0]) != `{"signature":"sig-raw","type":"thinking","thinking":"hmm"}` || string(blocks[1]) != `{"type":"text","text":"answer"}` {
		t.Fatalf("raw blocks = %s", blocks)
	}
	if usage == nil || usage.CacheReadTokens != 2 || usage.CacheWriteTokens != 1 || usage.UncachedTokens != 7 || usage.Estimated {
		t.Fatalf("usage = %+v", usage)
	}
}
