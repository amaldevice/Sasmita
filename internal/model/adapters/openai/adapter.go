// Package openai implements the OpenAI-compatible streaming wire format.
// All wire logic is confined to this adapter (ADR-0002).
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/amaldevice/sasmita/internal/model"
)

// Adapter talks to any OpenAI-compatible /chat/completions endpoint.
type Adapter struct {
	BaseURL    string
	APIKey     string
	ProviderID string
	Client     *http.Client
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// Field order is alphabetical so the frozen tool list is deterministic.
type wireFunction struct {
	Description string          `json:"description,omitempty"`
	Name        string          `json:"name"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type wireTool struct {
	Type     string      `json:"type"`
	Function wireFunction `json:"function"`
}

type wireToolCallFunction struct {
	Arguments string `json:"arguments"`
	Name      string `json:"name"`
}

type wireToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function wireToolCallFunction `json:"function"`
}

// wireMessage carries tool calls and tool results as first-class wire fields.
// Content is always present: null for an assistant tool-call turn.
type wireMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []wireToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

// requestBody field order is deliberate: messages is the final key so the
// serialization is append-only across turns (ADR-0001 golden prefix). Tools are
// frozen per conversation and sit before messages.
type requestBody struct {
	Model          string        `json:"model"`
	Stream         bool          `json:"stream"`
	StreamOptions  streamOptions `json:"stream_options"`
	PromptCacheKey string        `json:"prompt_cache_key,omitempty"`
	Tools          []wireTool    `json:"tools,omitempty"`
	Messages       []wireMessage `json:"messages"`
}

// canonicalRaw normalizes key order for generated JSON (tool schemas and tool
// arguments). Stored provider blocks are replayed verbatim and never pass here.
func canonicalRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return append(json.RawMessage(nil), raw...)
	}
	out, err := model.MarshalDeterministic(value)
	if err != nil {
		return append(json.RawMessage(nil), raw...)
	}
	return out
}

// EncodeRequest returns the exact bytes sent to the provider.
func EncodeRequest(req model.StreamRequest) ([]byte, error) {
	body := requestBody{
		Model:          req.Model,
		Stream:         true,
		StreamOptions:  streamOptions{IncludeUsage: true},
		PromptCacheKey: req.PromptCacheKey,
		Messages:       make([]wireMessage, 0, len(req.Messages)),
	}
	for _, tool := range req.Tools {
		body.Tools = append(body.Tools, wireTool{
			Type: "function",
			Function: wireFunction{
				Description: tool.Description,
				Name:        tool.Name,
				Parameters:  canonicalRaw(tool.InputSchema),
			},
		})
	}
	for _, m := range req.Messages {
		wm, err := wireMessageOf(m)
		if err != nil {
			return nil, err
		}
		body.Messages = append(body.Messages, wm)
	}
	return model.MarshalDeterministic(body)
}

// wireMessageOf keeps plain text as a JSON string and replays stored provider
// blocks verbatim. Neutral tool_use / tool_result blocks are mapped to the
// OpenAI wire fields so a tool-enabled conversation can continue across turns.
func wireMessageOf(m model.Message) (wireMessage, error) {
	wm := wireMessage{Role: m.Role}
	if len(m.Blocks) == 0 {
		content, err := model.MarshalDeterministic(m.Content)
		wm.Content = content
		return wm, err
	}
	var normal []json.RawMessage
	var calls []wireToolCall
	for _, raw := range m.Blocks {
		var probe struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			ToolUseID string          `json:"tool_use_id"`
			Name      string          `json:"name"`
			Input     json.RawMessage `json:"input"`
			Content   json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		switch probe.Type {
		case "tool_use":
			calls = append(calls, wireToolCall{
				ID: probe.ID, Type: "function",
				Function: wireToolCallFunction{
					Arguments: string(canonicalRaw(probe.Input)),
					Name:      probe.Name,
				},
			})
		case "tool_result":
			wm.Role = "tool"
			wm.ToolCallID = probe.ToolUseID
			var text string
			if len(probe.Content) > 0 && json.Unmarshal(probe.Content, &text) != nil {
				text = string(probe.Content)
			}
			encoded, err := model.MarshalDeterministic(text)
			if err != nil {
				return wireMessage{}, err
			}
			wm.Content = encoded
		default:
			normal = append(normal, append(json.RawMessage(nil), raw...))
		}
	}
	switch {
	case len(calls) > 0:
		wm.Content = json.RawMessage(`null`)
		wm.ToolCalls = calls
	case len(normal) > 0:
		encoded, err := model.MarshalDeterministic(normal)
		if err != nil {
			return wireMessage{}, err
		}
		wm.Content = encoded
	case len(wm.Content) == 0:
		// A tool_result already supplied the content; plain messages fall back
		// to their text field.
		encoded, err := model.MarshalDeterministic(m.Content)
		if err != nil {
			return wireMessage{}, err
		}
		wm.Content = encoded
	}
	return wm, nil
}

// CanonicalRequest is the append-only prefix view of the request. OpenAI-compatible
// providers cache automatically and have no explicit breakpoints, so the wire
// bytes and canonical bytes are identical.
func CanonicalRequest(req model.StreamRequest) ([]byte, error) {
	return EncodeRequest(req)
}

// StableRequestPrefix is the request body without its trailing `]}` envelope.
// A turn N+1 request extends these exact bytes with new messages; no earlier
// byte (including system instructions and the pinned cache key) changes.
func StableRequestPrefix(req model.StreamRequest) []byte {
	body, err := EncodeRequest(req)
	if err != nil {
		return nil
	}
	if len(body) < 2 {
		return body
	}
	return body[:len(body)-2]
}

func (a Adapter) Name() string {
	if a.ProviderID != "" {
		return a.ProviderID
	}
	return "openai"
}

// ProviderKind identifies the wire format so a model/provider switch can drop
// raw blocks that belong to another provider.
func (a Adapter) ProviderKind() string { return model.ProviderOpenAICompatible }

type wireChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int64 `json:"prompt_tokens"`
		CompletionTokens    int64 `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int64 `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CacheWriteTokens      int64 `json:"cache_write_tokens"`
		PromptCacheHitTokens  int64 `json:"prompt_cache_hit_tokens"`
		PromptCacheMissTokens int64 `json:"prompt_cache_miss_tokens"`
	} `json:"usage"`
	// llama.cpp reports its prompt cache in timings, outside usage.
	Timings struct {
		CacheN int64 `json:"cache_n"`
	} `json:"timings"`
}

type toolAccumulator struct {
	id, name, arguments string
}

// Stream performs one SSE request and normalizes text deltas, tool calls and
// usage. Cancellation flows from ctx into the HTTP request.
func (a Adapter) Stream(ctx context.Context, req model.StreamRequest, emit func(model.StreamEvent)) error {
	body, err := EncodeRequest(req)
	if err != nil {
		return err
	}
	url := strings.TrimRight(a.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if a.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.APIKey)
	}
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("provider returned %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	pendingTools := map[int]*toolAccumulator{}
	var order []int
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			break
		}
		var chunk wireChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("invalid SSE chunk: %w", err)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				emit(model.StreamEvent{Kind: model.EventTextDelta, Text: choice.Delta.Content})
			}
			for _, call := range choice.Delta.ToolCalls {
				acc := pendingTools[call.Index]
				if acc == nil {
					acc = &toolAccumulator{}
					pendingTools[call.Index] = acc
					order = append(order, call.Index)
				}
				if call.ID != "" {
					acc.id = call.ID
				}
				if call.Function.Name != "" {
					acc.name = call.Function.Name
				}
				acc.arguments += call.Function.Arguments
			}
		}
		if chunk.Usage != nil {
			u := model.Usage{
				ProviderID:   a.Name(),
				ModelID:      req.Model,
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
			switch {
			case chunk.Usage.PromptCacheHitTokens > 0 || chunk.Usage.PromptCacheMissTokens > 0:
				// DeepSeek reports hits and misses separately.
				u.CacheReadTokens = chunk.Usage.PromptCacheHitTokens
				u.UncachedTokens = chunk.Usage.PromptCacheMissTokens
			default:
				u.CacheReadTokens = chunk.Usage.PromptTokensDetails.CachedTokens
			}
			u.CacheWriteTokens = chunk.Usage.CacheWriteTokens
			if chunk.Timings.CacheN > 0 && u.CacheReadTokens == 0 {
				u.CacheReadTokens = chunk.Timings.CacheN
			}
			normalized := model.FinalizeUsage(u)
			emit(model.StreamEvent{Kind: model.EventUsage, Usage: &normalized})
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// Tool-call arguments arrive as string fragments. Accumulate them into one
	// call per index, in index order, and emit the canonical neutral block that
	// the agent appends to history (ADR-0001, append-only across turns).
	if len(order) > 0 {
		sort.Ints(order)
		blocks := make([]json.RawMessage, 0, len(order))
		for _, index := range order {
			acc := pendingTools[index]
			arguments := canonicalRaw(json.RawMessage(acc.arguments))
			emit(model.StreamEvent{Kind: model.EventToolUse, ToolCall: &model.ToolCall{
				ID: acc.id, Name: acc.name, Input: arguments,
			}})
			block, err := model.MarshalDeterministic(struct {
				Type  string          `json:"type"`
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			}{Type: "tool_use", ID: acc.id, Name: acc.name, Input: arguments})
			if err != nil {
				return err
			}
			blocks = append(blocks, block)
		}
		emit(model.StreamEvent{Kind: model.EventContentBlocks, ContentBlocks: blocks})
	}
	return nil
}
