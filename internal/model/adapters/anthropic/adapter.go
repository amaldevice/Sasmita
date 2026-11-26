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
	Type         string       `json:"type"`
	Text         string       `json:"text"`
	CacheControl cacheControl `json:"cache_control"`
}

type wireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
	Tools     []wireTool    `json:"tools,omitempty"`
	System    []systemBlock `json:"system,omitempty"`
	Messages  []wireMessage `json:"messages"`
}

// EncodeRequest returns the exact bytes sent to the provider. A leading
// "system" message becomes the static system block with a 1h cache breakpoint;
// all remaining roles are passed through unchanged in append-only order.
func EncodeRequest(req model.StreamRequest) ([]byte, error) {
	body := requestBody{
		Model:     req.Model,
		MaxTokens: defaultMaxTokens,
		Stream:    true,
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
			body.System = append(body.System, systemBlock{
				Type: "text", Text: m.Content,
				CacheControl: cacheControl{Type: "ephemeral", TTL: "1h"},
			})
			continue
		}
		body.Messages = append(body.Messages, wireMessage{Role: m.Role, Content: m.Content})
	}
	return json.Marshal(body)
}

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
	} `json:"message"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
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
			emit(model.StreamEvent{Kind: model.EventUsage, Usage: &usage})
			usageEmitted = true
		}
	}
	pendingTools := map[int]*model.ToolCall{}

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
			if ev.Message != nil && ev.Message.Usage != nil {
				u := ev.Message.Usage
				usage = model.Usage{
					ProviderID:       a.Name(),
					ModelID:          req.Model,
					InputTokens:      u.InputTokens,
					CacheReadTokens:  u.CacheReadInputTokens,
					CacheWriteTokens: u.CacheCreationInputTokens,
				}
				usageSeen = true
			}
		case "content_block_start":
			if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
				pendingTools[ev.Index] = &model.ToolCall{
					ID: ev.ContentBlock.ID, Name: ev.ContentBlock.Name,
					Input: json.RawMessage(`{}`),
				}
			}
		case "content_block_delta":
			if ev.Delta == nil {
				continue
			}
			switch ev.Delta.Type {
			case "text_delta":
				if ev.Delta.Text != "" {
					emit(model.StreamEvent{Kind: model.EventTextDelta, Text: ev.Delta.Text})
				}
			case "input_json_delta":
				if tc := pendingTools[ev.Index]; tc != nil && ev.Delta.PartialJSON != "" {
					tc.Input = append(tc.Input, ev.Delta.PartialJSON...)
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
	emitUsage()
	return scanner.Err()
}
