package discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
)

// Validate checks model-supplied arguments against a tool's JSON Schema before
// anything executes. Strict provider decoding is not available for every model,
// so the check is done in Go at the dispatcher boundary (ADR-0003).
func Validate(schema json.RawMessage, args json.RawMessage) error {
	var s schemaNode
	if err := json.Unmarshal(schema, &s); err != nil {
		return fmt.Errorf("invalid tool schema: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return fmt.Errorf("invalid JSON arguments: %w", err)
	}
	if dec.More() {
		return fmt.Errorf("invalid JSON arguments: trailing value")
	}
	return s.check(value, "args")
}

type schemaNode struct {
	Type                 string                `json:"type"`
	Required             []string              `json:"required"`
	Properties           map[string]schemaNode `json:"properties"`
	AdditionalProperties any                   `json:"additionalProperties"`
	Items                *schemaNode           `json:"items"`
	Enum                 []any                 `json:"enum"`
	Const                any                   `json:"const"`
	Pattern              string                `json:"pattern"`
	Minimum              *float64              `json:"minimum"`
	Maximum              *float64              `json:"maximum"`
	MinItems             *int                  `json:"minItems"`
}

func (s schemaNode) check(value any, path string) error {
	if s.Const != nil {
		if fmt.Sprint(s.Const) != fmt.Sprint(value) {
			return fmt.Errorf("%s must be %v", path, s.Const)
		}
	}
	if len(s.Enum) > 0 {
		ok := false
		for _, option := range s.Enum {
			if fmt.Sprint(option) == fmt.Sprint(value) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("%s must be one of %v", path, s.Enum)
		}
	}
	switch s.Type {
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		for _, key := range s.Required {
			if _, present := obj[key]; !present {
				return fmt.Errorf("%s.%s is required", path, key)
			}
		}
		if allowed, ok := s.AdditionalProperties.(bool); ok && !allowed {
			for key := range obj {
				if _, known := s.Properties[key]; !known {
					return fmt.Errorf("%s.%s is not allowed", path, key)
				}
			}
		}
		for key, child := range s.Properties {
			v, present := obj[key]
			if !present {
				continue
			}
			if err := child.check(v, path+"."+key); err != nil {
				return err
			}
		}
	case "array":
		arr, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		if s.MinItems != nil && len(arr) < *s.MinItems {
			return fmt.Errorf("%s needs at least %d items", path, *s.MinItems)
		}
		if s.Items != nil {
			for i, item := range arr {
				if err := s.Items.check(item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	case "string":
		str, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", path)
		}
		if s.Pattern != "" {
			re, err := regexp.Compile(s.Pattern)
			if err != nil || !re.MatchString(str) {
				return fmt.Errorf("%s does not match %s", path, s.Pattern)
			}
		}
	case "number", "integer":
		num, ok := numeric(value)
		if !ok {
			return fmt.Errorf("%s must be a %s", path, s.Type)
		}
		if s.Type == "integer" && num != float64(int64(num)) {
			return fmt.Errorf("%s must be an integer", path)
		}
		if s.Minimum != nil && num < *s.Minimum {
			return fmt.Errorf("%s is below %v", path, *s.Minimum)
		}
		if s.Maximum != nil && num > *s.Maximum {
			return fmt.Errorf("%s exceeds %v", path, *s.Maximum)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	}
	return nil
}

func numeric(value any) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		n, err := strconv.ParseFloat(string(v), 64)
		return n, err == nil
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}
