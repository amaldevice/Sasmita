package onboarding_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amaldevice/sasmita/internal/onboarding"
)

// Detection only offers endpoints that actually answered a protocol-aware
// probe. A port that is open but speaks something else must never be offered.
func TestDetectLocalEndpointsOffersOnlyEndpointsThatAnswered(t *testing.T) {
	probe := func(_ context.Context, endpoint onboarding.LocalEndpoint) error {
		if endpoint.Kind == onboarding.LlamaCPPKind {
			return nil
		}
		return errors.New("connection refused")
	}

	got := onboarding.DetectLocalEndpoints(context.Background(), probe)
	if len(got) != 1 {
		t.Fatalf("got %d endpoints, want 1: %+v", len(got), got)
	}
	if got[0].Kind != onboarding.LlamaCPPKind {
		t.Fatalf("offered %q, want %q", got[0].Kind, onboarding.LlamaCPPKind)
	}
	if got[0].BaseURL != "http://127.0.0.1:8080" || got[0].APIBaseURL != "http://127.0.0.1:8080/v1" {
		t.Fatalf("unexpected URLs: %+v", got[0])
	}
}

func TestDetectLocalEndpointsKeepsKnownOrderAndHandlesEmpty(t *testing.T) {
	ctx := context.Background()
	all := onboarding.DetectLocalEndpoints(ctx, func(context.Context, onboarding.LocalEndpoint) error { return nil })
	if len(all) != 2 || all[0].Kind != onboarding.OllamaKind || all[1].Kind != onboarding.LlamaCPPKind {
		t.Fatalf("detected endpoints out of order: %+v", all)
	}

	none := onboarding.DetectLocalEndpoints(ctx, func(context.Context, onboarding.LocalEndpoint) error {
		return errors.New("offline")
	})
	if len(none) != 0 {
		t.Fatalf("offline endpoints were offered: %+v", none)
	}
}

func TestHTTPProbeRequiresExpectedWireResponse(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer ollama.Close()

	llama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer llama.Close()

	bogus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer bogus.Close()

	probe := onboarding.HTTPProbe(nil)
	checks := []struct {
		name     string
		endpoint onboarding.LocalEndpoint
		wantOK   bool
	}{
		{"ollama tags", onboarding.LocalEndpoint{Kind: onboarding.OllamaKind, BaseURL: ollama.URL}, true},
		{"llama health", onboarding.LocalEndpoint{Kind: onboarding.LlamaCPPKind, BaseURL: llama.URL}, true},
		{"wrong ollama shape", onboarding.LocalEndpoint{Kind: onboarding.OllamaKind, BaseURL: bogus.URL}, false},
		{"wrong llama shape", onboarding.LocalEndpoint{Kind: onboarding.LlamaCPPKind, BaseURL: bogus.URL}, false},
		{"swapped protocols", onboarding.LocalEndpoint{Kind: onboarding.OllamaKind, BaseURL: llama.URL}, false},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := probe(context.Background(), check.endpoint)
			if (err == nil) != check.wantOK {
				t.Fatalf("probe error = %v, wantOK = %v", err, check.wantOK)
			}
		})
	}
}
