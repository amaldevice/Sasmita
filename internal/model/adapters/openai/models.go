package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/amaldevice/sasmita/internal/model"
)

// ListModels reads provider metadata. Providers that do not publish capability
// fields yield "unknown" rather than a value guessed from the model name.
func (a Adapter) ListModels(ctx context.Context) ([]model.ModelInfo, error) {
	url := strings.TrimRight(a.BaseURL, "/") + "/v1/models"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if a.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.APIKey)
	}
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrUnreachable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%w: provider returned %s: %s", model.ErrUnauthorized, resp.Status, strings.TrimSpace(string(msg)))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("provider returned %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var payload struct {
		Data []struct {
			ID             string `json:"id"`
			ContextWindow  *int64 `json:"context_window"`
			ContextLength  *int64 `json:"context_length"`
			SupportsTools  *bool  `json:"supports_tools"`
			SupportsVision *bool  `json:"supports_vision"`
			Vision         *bool  `json:"vision"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("invalid model list: %w", err)
	}
	out := make([]model.ModelInfo, 0, len(payload.Data))
	for _, m := range payload.Data {
		window := m.ContextWindow
		if window == nil {
			window = m.ContextLength
		}
		vision := m.SupportsVision
		if vision == nil {
			vision = m.Vision
		}
		out = append(out, model.ModelInfo{
			ID: m.ID,
			Capabilities: model.ModelCapabilities{
				Tools:         model.BoolCapability(m.SupportsTools),
				Vision:        model.BoolCapability(vision),
				ContextWindow: window,
			},
		})
	}
	return out, nil
}
