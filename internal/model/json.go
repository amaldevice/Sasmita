package model

import (
	"bytes"
	"encoding/json"
)

// MarshalDeterministic serializes with a fixed field order and without HTML
// escaping, so a raw provider content block survives storage and replay with
// its original bytes intact (ADR-0001 byte-stable history).
func MarshalDeterministic(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
