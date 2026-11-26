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

type wireMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// requestBody field order is deliberate: messages is the final key so the
// serialization is append-only across turns (ADR-0001 golden prefix).
type requestBody struct {
	Model          string        `json:"model"`
	Stream         bool          `json:"stream"`
	StreamOptions  streamOptions `json:"stream_options"`
	PromptCacheKey string        `json:"prompt_cache_key,omitempty"`
	Messages       []wireMessage `json:"messages"`
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
	for _, m := range req.Messages {
		content, err := wireContent(m)
		if err != nil {
			return nil, err
		}
		body.Messages = append(body.Messages, wireMessage{Role: m.Role, Content: content})
	}
	return model.MarshalDeterministic(body)
}

// wireContent keeps plain text as a JSON string and replays stored provider
// blocks verbatim as a JSON array (deterministic key order, unchanged bytes).
func wireContent(m model.Message) (json.RawMessage, error) {
	if len(m.Blocks) == 0 {
		return model.MarshalDeterministic(m.Content)
	}
	return model.MarshalDeterministic(m.Blocks)
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
			Content string `json:"content"`
		} `json:"delta"`
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

// Stream performs one SSE request and normalizes text deltas and usage.
// Cancellation flows from ctx into the HTTP request.
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
	return scanner.Err()
}
