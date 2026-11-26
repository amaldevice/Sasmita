package integration_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/amaldevice/sasmita/internal/domain"
)

// query_table belongs to the out-of-scope Phase 4 Mode Analis Data. It must not
// appear in find_tools results, and call_tool must fail closed when asked for it.
func TestSeam1QueryTableIsOutOfScope(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		switch calls.Add(1) {
		case 1:
			fmt.Fprint(w, discoverySSE("Mencari. ", "scope-1", "find_tools", `{"query":"query table rows"}`))
		case 2:
			fmt.Fprint(w, discoverySSE("Memanggil. ", "scope-2", "call_tool", `{"name":"query_table","args":{"rows":[{"v":2}],"column":"v","op":"sum"}}`))
		default:
			fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"Selesai."}]`,
				`{"input_tokens":10,"output_tokens":2,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}`, "Selesai."))
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
	found, failed, executed := "", "", false
	for _, ev := range events {
		if ev.Type != domain.EventToolExecuted && ev.Type != domain.EventToolFailed {
			continue
		}
		p := payloadOf(t, ev)
		name, _ := p["name"].(string)
		output, _ := p["output"].(string)
		errText, _ := p["error"].(string)
		if ev.Type == domain.EventToolFailed && name == "call_tool" {
			failed = errText
		}
		if ev.Type == domain.EventToolExecuted {
			if name == "query_table" {
				executed = true
			}
			if name == "find_tools" {
				found = output
			}
		}
	}
	if strings.Contains(found, "query_table") {
		t.Fatalf("find_tools exposed the out-of-scope tool: %s", found)
	}
	if executed {
		t.Fatal("query_table executed in a v1 Mode")
	}
	if !strings.Contains(failed, "not enabled") {
		t.Fatalf("call_tool did not fail closed for query_table: %q", failed)
	}
}
