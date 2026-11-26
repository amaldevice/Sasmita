// Package onboarding owns first-run detection of local inference endpoints and
// the catalog of example prompts shown in an empty chat. It has no dependency
// on the desktop shell or on provider plumbing.
package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Known local endpoints. Ollama exposes /api/tags; llama.cpp server exposes
// /health. APIBaseURL is the OpenAI-compatible prefix used when a detected
// endpoint is turned into a provider profile.
const (
	OllamaKind   = "ollama"
	LlamaCPPKind = "llama.cpp"
)

type LocalEndpoint struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	BaseURL    string `json:"baseUrl"`
	APIBaseURL string `json:"apiBaseUrl"`
}

var KnownLocalEndpoints = []LocalEndpoint{
	{
		Kind: OllamaKind, Name: "Ollama",
		BaseURL: "http://127.0.0.1:11434", APIBaseURL: "http://127.0.0.1:11434/v1",
	},
	{
		Kind: LlamaCPPKind, Name: "llama.cpp",
		BaseURL: "http://127.0.0.1:8080", APIBaseURL: "http://127.0.0.1:8080/v1",
	},
}

// Probe answers whether an endpoint is running. Tests inject a fake; the
// desktop shell uses HTTPProbe.
type Probe func(ctx context.Context, endpoint LocalEndpoint) error

// DetectLocalEndpoints probes every known candidate in parallel and returns
// only the ones that answered, in known-endpoint order. A failed probe is not
// an error: it simply means nothing is offered.
func DetectLocalEndpoints(ctx context.Context, probe Probe) []LocalEndpoint {
	if probe == nil {
		probe = HTTPProbe(nil)
	}
	answered := make([]LocalEndpoint, len(KnownLocalEndpoints))
	var wg sync.WaitGroup
	for i, endpoint := range KnownLocalEndpoints {
		wg.Add(1)
		go func(i int, endpoint LocalEndpoint) {
			defer wg.Done()
			if err := probe(ctx, endpoint); err == nil {
				answered[i] = endpoint
			}
		}(i, endpoint)
	}
	wg.Wait()

	out := make([]LocalEndpoint, 0, len(answered))
	for _, endpoint := range answered {
		if endpoint.Kind != "" {
			out = append(out, endpoint)
		}
	}
	return out
}

// HTTPProbe checks the endpoint's own protocol shape rather than just opening
// a socket, so an unrelated service on a known port is never offered as a
// running model. The deadline is short enough for a first-run screen.
func HTTPProbe(client *http.Client) Probe {
	if client == nil {
		client = &http.Client{
			Timeout:   700 * time.Millisecond,
			Transport: &http.Transport{Proxy: nil},
		}
	}
	return func(ctx context.Context, endpoint LocalEndpoint) error {
		path := "/health"
		if endpoint.Kind == OllamaKind {
			path = "/api/tags"
		}
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			strings.TrimRight(endpoint.BaseURL, "/")+path, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s returned status %d", endpoint.Name, resp.StatusCode)
		}

		var body map[string]json.RawMessage
		if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil {
			return fmt.Errorf("%s returned a malformed response: %w", endpoint.Name, err)
		}
		switch endpoint.Kind {
		case OllamaKind:
			if _, ok := body["models"]; !ok {
				return errors.New("ollama response has no models field")
			}
		case LlamaCPPKind:
			var status string
			if err := json.Unmarshal(body["status"], &status); err != nil || status != "ok" {
				return errors.New("llama.cpp server is not healthy")
			}
		default:
			return fmt.Errorf("unknown local endpoint kind %q", endpoint.Kind)
		}
		return nil
	}
}
