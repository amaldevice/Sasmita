package desktop_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/desktop"
)

// GLOSSARY.md lists "harness" among the avoided synonyms for Mode. The typed
// IPC surface is the wire boundary, so the old term must not leak into it.
func TestModeTerminologyOnWire(t *testing.T) {
	conversation, err := json.Marshal(desktop.ConversationDTO{ActiveModeID: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(conversation), `"activeModeId":"chat"`) {
		t.Fatalf("conversation DTO has no activeModeId tag: %s", conversation)
	}
	request, err := json.Marshal(desktop.StartRunRequestDTO{ModeID: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(request), `"modeId":"chat"`) {
		t.Fatalf("start-run DTO has no modeId tag: %s", request)
	}
	for _, encoded := range []string{string(conversation), string(request)} {
		if strings.Contains(strings.ToLower(encoded), "harness") {
			t.Fatalf("avoided term reached the wire: %s", encoded)
		}
	}
}
