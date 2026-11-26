// Package anthropic implements the Anthropic-compatible Messages wire format.
// All wire logic is confined to this adapter (ADR-0002). The request layout is
// append-only: frozen tools, static system, then byte-identical history, so a
// new turn extends earlier bytes instead of rewriting them (ADR-0001).
package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/amaldevice/sasmita/internal/model"
)

const (
	apiVersion       = "2023-06-01"
	defaultMaxTokens = 1024
)

// Adapter talks to any Anthropic-compatible /v1/messages endpoint.
type Adapter struct {
	BaseURL    string
	APIKey     string
	ProviderID string
	Client     *http.Client
	// MaxAttempts bounds retries; zero means 3. Sleep is injectable so tests
	// can assert Retry-After handling without actually waiting.
	MaxAttempts int
	Sleep       func(context.Context, time.Duration) error
}

func defaultSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func capDelay(d time.Duration) time.Duration {
	if d > 30*time.Second {
		return 30 * time.Second
	}
	return d
}

// retryDelay honors Retry-After (seconds or HTTP date) and falls back to
// bounded exponential backoff with jitter (TAD §9.3).
func retryDelay(header string, attempt int) time.Duration {
	if v, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && v >= 0 {
		return capDelay(time.Duration(v) * time.Second)
	}
	if t, err := http.ParseTime(header); err == nil {
		if d := time.Until(t); d > 0 {
			return capDelay(d)
		}
	}
	base := time.Duration(1<<attempt) * 100 * time.Millisecond
	return capDelay(base + time.Duration(rand.Int64N(int64(base/2)+1)))
}

// do performs the request with bounded retries. Credential failures are not
// retried; transport failures are classified as unreachable. Only pre-stream
// status codes are retried, so a retry can never duplicate emitted deltas.
func (a Adapter) do(ctx context.Context, httpReq *http.Request) (*http.Response, error) {
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	sleep := a.Sleep
	if sleep == nil {
		sleep = defaultSleep
	}
	attempts := a.MaxAttempts
	if attempts <= 0 {
		attempts = 3
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 && httpReq.GetBody != nil {
			body, err := httpReq.GetBody()
			if err != nil {
				return nil, err
			}
			httpReq.Body = body
		}
		resp, err := client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", model.ErrUnreachable, err)
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			return nil, fmt.Errorf("%w: provider returned %s: %s", model.ErrUnauthorized, resp.Status, strings.TrimSpace(string(msg)))
		}
		if resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			lastErr = fmt.Errorf("provider returned %s: %s", resp.Status, strings.TrimSpace(string(msg)))
			if i == attempts-1 {
				break
			}
			if err := sleep(ctx, retryDelay(resp.Header.Get("Retry-After"), i)); err != nil {
				return nil, err
			}
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

type cacheControl struct {
	Type string `json:"type"`
	TTL  string `json:"ttl"`
}

type systemBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type wireMessage struct {
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
}

type wireTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// requestBody field order is deliberate. messages is the final key so the
// serialized prefix is append-only across turns (ADR-0001 golden prefix).
type requestBody struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Stream    bool          `json:"stream"`
	SessionID string        `json:"session_id,omitempty"`
	Tools     []wireTool    `json:"tools,omitempty"`
	System    []systemBlock `json:"system,omitempty"`
	Messages  []wireMessage `json:"messages"`
}

// encode builds either the wire request (with cache breakpoints) or the
// canonical byte-stable view (without them). Breakpoints are wire decorations,
// never stored in canonical history — see docs/benchmarks/s2-adapters-cache.md.
func encode(req model.StreamRequest, breakpoints bool) ([]byte, error) {
	body := requestBody{
		Model:     req.Model,
		MaxTokens: defaultMaxTokens,
		Stream:    true,
		SessionID: req.PromptCacheKey,
		Messages:  make([]wireMessage, 0, len(req.Messages)),
	}
	for _, tool := range req.Tools {
		schema := tool.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{}`)
		}
		body.Tools = append(body.Tools, wireTool{
			Name: tool.Name, Description: tool.Description, InputSchema: schema,
		})
	}
	for _, m := range req.Messages {
		if m.Role == "system" {
			block := systemBlock{Type: "text", Text: m.Content}
			if breakpoints {
				block.CacheControl = &cacheControl{Type: "ephemeral", TTL: "1h"}
			}
			body.System = append(body.System, block)
			continue
		}
		wm := wireMessage{Role: m.Role, Content: make([]json.RawMessage, 0, max(1, len(m.Blocks)))}
		if len(m.Blocks) == 0 {
			text, err := model.MarshalDeterministic(struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{Type: "text", Text: m.Content})
			if err != nil {
				return nil, err
			}
			wm.Content = append(wm.Content, text)
		} else {
			for _, raw := range m.Blocks {
				wm.Content = append(wm.Content, append(json.RawMessage(nil), raw...))
			}
		}
		body.Messages = append(body.Messages, wm)
	}
	if breakpoints {
		type location struct{ message, block int }
		var locations []location
		for mi := range body.Messages {
			for bi := range body.Messages[mi].Content {
				locations = append(locations, location{mi, bi})
			}
		}
		apply := func(loc location, ttl string) {
			block := body.Messages[loc.message].Content[loc.block]
			body.Messages[loc.message].Content[loc.block] = withCacheControl(block, ttl)
		}
		if len(locations) > 0 {
			apply(locations[len(locations)-1], "5m")
			// Anthropic looks back only 20 blocks; a turn that adds more needs
			// an extra breakpoint near the end of the stable prefix.
			if len(locations) > 20 {
				apply(locations[len(locations)-20], "5m")
			}
		}
	}
	return model.MarshalDeterministic(body)
}

// withCacheControl appends a marker to a raw block without re-serializing any
// of the provider's original bytes or changing its key order.
func withCacheControl(block json.RawMessage, ttl string) json.RawMessage {
	b := bytes.TrimSpace(block)
	if len(b) < 2 || b[0] != '{' || b[len(b)-1] != '}' {
		return block
	}
	out := make([]byte, 0, len(b)+72)
	out = append(out, b[:len(b)-1]...)
	out = append(out, `,"cache_control":{"type":"ephemeral","ttl":"`...)
	out = append(out, ttl...)
	out = append(out, `"}`...)
	out = append(out, '}')
	return out
}

// EncodeRequest returns the exact bytes sent to the provider, including cache
// breakpoints: 1h at the end of the static system block, 5m on the last block,
// and a third only when one turn adds more than 20 blocks.
func EncodeRequest(req model.StreamRequest) ([]byte, error) { return encode(req, true) }

// CanonicalRequest returns the breakpoint-free, byte-stable view of the same
// request. Appending a turn extends these bytes without rewriting earlier ones.
func CanonicalRequest(req model.StreamRequest) ([]byte, error) { return encode(req, false) }

func (a Adapter) Name() string {
	if a.ProviderID != "" {
		return a.ProviderID
	}
	return "anthropic"
}

type streamEvent struct {
	Type    string `json:"type"`
	Message *struct {
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
		// Content may carry the finished blocks verbatim; when present it is
		// stored byte-for-byte instead of being rebuilt from deltas.
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		Signature   string `json:"signature"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
	Index int `json:"index"`
	Usage *struct {
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Stream performs one SSE request and normalizes text deltas, tool calls and
// usage. Cancellation flows from ctx into the HTTP request.
func (a Adapter) Stream(ctx context.Context, req model.StreamRequest, emit func(model.StreamEvent)) error {
	body, err := EncodeRequest(req)
	if err != nil {
		return err
	}
	url := strings.TrimRight(a.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Anthropic-Version", apiVersion)
	if a.APIKey != "" {
		httpReq.Header.Set("X-Api-Key", a.APIKey)
	}
	resp, err := a.do(ctx, httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("provider returned %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	var usage model.Usage
	usageSeen, usageEmitted := false, false
	emitUsage := func() {
		if usageSeen && !usageEmitted {
			usage = model.FinalizeUsage(usage)
			emit(model.StreamEvent{Kind: model.EventUsage, Usage: &usage})
			usageEmitted = true
		}
	}
	pendingTools := map[int]*model.ToolCall{}
	type blockState struct {
		typ, text, thinking, signature, id, name, input string
	}
	blockStates := map[int]*blockState{}
	var rawBlocks []json.RawMessage

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var ev streamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return fmt.Errorf("invalid SSE data: %w", err)
		}
		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				if ev.Message.Usage != nil {
					u := ev.Message.Usage
					usage = model.Usage{
						ProviderID:       a.Name(),
						ModelID:          req.Model,
						InputTokens:      u.InputTokens,
						CacheReadTokens:  u.CacheReadInputTokens,
						CacheWriteTokens: u.CacheCreationInputTokens,
						// Anthropic's input_tokens already excludes cached tokens.
						UncachedTokens: u.InputTokens,
					}
					usageSeen = true
				}
				if len(ev.Message.Content) > 0 {
					var blocks []json.RawMessage
					if json.Unmarshal(ev.Message.Content, &blocks) == nil {
						rawBlocks = blocks
					}
				}
			}
		case "content_block_start":
			if ev.ContentBlock != nil {
				blockStates[ev.Index] = &blockState{typ: ev.ContentBlock.Type}
				if ev.ContentBlock.Type == "tool_use" {
					state := blockStates[ev.Index]
					state.id, state.name, state.input = ev.ContentBlock.ID, ev.ContentBlock.Name, "{}"
					pendingTools[ev.Index] = &model.ToolCall{
						ID: ev.ContentBlock.ID, Name: ev.ContentBlock.Name,
						Input: json.RawMessage(`{}`),
					}
				}
			}
		case "content_block_delta":
			if ev.Delta == nil {
				continue
			}
			state := blockStates[ev.Index]
			if state == nil {
				state = &blockState{}
				blockStates[ev.Index] = state
			}
			switch ev.Delta.Type {
			case "text_delta":
				state.text += ev.Delta.Text
				if ev.Delta.Text != "" {
					emit(model.StreamEvent{Kind: model.EventTextDelta, Text: ev.Delta.Text})
				}
			case "thinking_delta":
				state.thinking += ev.Delta.Thinking
			case "signature_delta":
				state.signature += ev.Delta.Signature
			case "input_json_delta":
				if ev.Delta.PartialJSON != "" {
					state.input += ev.Delta.PartialJSON
					if tc := pendingTools[ev.Index]; tc != nil {
						tc.Input = append(tc.Input, ev.Delta.PartialJSON...)
					}
				}
			}
		case "content_block_stop":
			if tc := pendingTools[ev.Index]; tc != nil {
				emit(model.StreamEvent{Kind: model.EventToolUse, ToolCall: tc})
				delete(pendingTools, ev.Index)
			}
		case "message_delta":
			if ev.Usage != nil {
				usage.OutputTokens = ev.Usage.OutputTokens
				usageSeen = true
			}
			emitUsage()
		case "message_stop":
			emitUsage()
		case "error":
			if ev.Error != nil && ev.Error.Message != "" {
				return fmt.Errorf("provider stream error: %s", ev.Error.Message)
			}
			return errors.New("provider stream error")
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	// Rebuild streamed blocks only when the provider did not supply the raw
	// array. Key order is fixed so tool_use serialization stays deterministic.
	if len(rawBlocks) == 0 && len(blockStates) > 0 {
		indices := make([]int, 0, len(blockStates))
		for index := range blockStates {
			indices = append(indices, index)
		}
		sort.Ints(indices)
		for _, index := range indices {
			state := blockStates[index]
			var block []byte
			switch state.typ {
			case "thinking":
				block, _ = model.MarshalDeterministic(struct {
					Type      string `json:"type"`
					Thinking  string `json:"thinking"`
					Signature string `json:"signature"`
				}{Type: "thinking", Thinking: state.thinking, Signature: state.signature})
			case "tool_use":
				input := json.RawMessage(state.input)
				if len(input) == 0 {
					input = json.RawMessage(`{}`)
				}
				block, _ = model.MarshalDeterministic(struct {
					Type  string          `json:"type"`
					ID    string          `json:"id"`
					Name  string          `json:"name"`
					Input json.RawMessage `json:"input"`
				}{Type: "tool_use", ID: state.id, Name: state.name, Input: input})
			default:
				block, _ = model.MarshalDeterministic(struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{Type: "text", Text: state.text})
			}
			rawBlocks = append(rawBlocks, block)
		}
	}
	if len(rawBlocks) > 0 {
		emit(model.StreamEvent{Kind: model.EventContentBlocks, ContentBlocks: rawBlocks})
	}
	emitUsage()
	return nil
}
