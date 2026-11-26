package integration_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/desktop"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model/adapters/openai"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

type memorySecrets struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *memorySecrets) Put(_ context.Context, ref, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[ref] = value
	return nil
}
func (s *memorySecrets) Get(_ context.Context, ref string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[ref], nil
}
func (s *memorySecrets) Delete(_ context.Context, ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, ref)
	return nil
}

func providerFixture(t *testing.T) (*repositories.Store, *appsvc.ProviderService, *appsvc.Service, *desktop.Service) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(t.TempDir() + "/sasmita.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	now := time.Now()
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	secrets := &memorySecrets{values: map[string]string{}}
	providers := appsvc.NewProviderService(store, appsvc.ProviderServiceConfig{Secrets: secrets, Now: func() time.Time { return now }})
	// A default adapter that must never be used when a profile is requested.
	unused := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("default adapter was used despite a profile ID")
	}))
	defer unused.Close()
	svc := appsvc.New(store, appsvc.Config{
		Provider:     openai.Adapter{BaseURL: unused.URL, ProviderID: "default"},
		DefaultModel: "m", NewID: newIDGenerator(),
	})
	svc.SetProviderResolver(providers.AdapterFor)
	return store, providers, svc, desktop.NewServiceWithProviders(svc, providers)
}

// Seam 1: a StartRun carrying a provider profile ID reaches that profile's base
// URL with its Keychain credential, for both wire formats.
func TestSeam1ProfileIDSelectsAdapterAndSecret(t *testing.T) {
	ctx := context.Background()
	_, providers, _, bindings := providerFixture(t)

	cases := []struct {
		name   string
		kind   domain.ProviderKind
		key    string
		header string
		secret string
		script func(http.ResponseWriter)
	}{
		{
			name: "openai-compatible", kind: domain.ProviderOpenAICompatible,
			key: "sk-openai", header: "Authorization", secret: "Bearer sk-openai",
			script: func(w http.ResponseWriter) { writeSSE(w, "openai answer") },
		},
		{
			name: "anthropic-compatible", kind: domain.ProviderAnthropicCompatible,
			key: "sk-ant", header: "X-Api-Key", secret: "sk-ant",
			script: func(w http.ResponseWriter) {
				fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"anthropic answer"}]`,
					`{"input_tokens":5,"output_tokens":2}`, "anthropic answer"))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var headers []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				mu.Lock()
				headers = append(headers, r.Header.Get(tc.header))
				mu.Unlock()
				w.Header().Set("Content-Type", "text/event-stream")
				tc.script(w)
			}))
			defer srv.Close()
			profile, err := providers.AddProviderProfile(ctx, domain.AddProviderProfileRequest{
				Kind: tc.kind, DisplayName: tc.name, BaseURL: srv.URL, APIKey: tc.key,
			})
			if err != nil {
				t.Fatal(err)
			}
			conv, err := bindings.CreateConversation(ctx, "p1", "Profile", "chat")
			if err != nil {
				t.Fatal(err)
			}
			handle, err := bindings.StartRun(ctx, desktop.StartRunRequestDTO{
				ConversationID: conv.ID, Text: "hello", ModelRef: "m", ModeID: "chat", ProviderID: profile.ID,
			})
			if err != nil {
				t.Fatal(err)
			}
			for range handle.Events {
			}
			mu.Lock()
			defer mu.Unlock()
			if len(headers) == 0 || headers[0] != tc.secret {
				t.Fatalf("credential header = %v, want %q", headers, tc.secret)
			}
			run, err := bindings.ListRuns(ctx, conv.ID)
			if err != nil || len(run) != 1 || run[0].Status != "completed" {
				t.Fatalf("run = %+v (err %v)", run, err)
			}
		})
	}
}

// Seam 1: switching to a different provider drops the old provider's raw
// blocks from the derived active context without rewriting canonical history.
func TestSeam1ProfileSwitchDropsForeignBlocks(t *testing.T) {
	ctx := context.Background()
	store, providers, _, bindings := providerFixture(t)

	const thinking = `{"type":"thinking","signature":"sig-profile","thinking":"hidden"}`
	var anthropicCalls int
	anthropicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		anthropicCalls++
		w.Header().Set("Content-Type", "text/event-stream")
		if anthropicCalls == 1 {
			fmt.Fprint(w, anthropicStreamWithBlocks(`[`+thinking+`,{"type":"text","text":"first"}]`, `{"input_tokens":5,"output_tokens":1}`, "first"))
			return
		}
		fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"second"}]`, `{"input_tokens":5,"output_tokens":1}`, "second"))
	}))
	defer anthropicServer.Close()
	var openaiBody []byte
	openaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openaiBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, "third")
	}))
	defer openaiServer.Close()

	from, err := providers.AddProviderProfile(ctx, domain.AddProviderProfileRequest{
		Kind: domain.ProviderAnthropicCompatible, DisplayName: "A", BaseURL: anthropicServer.URL, APIKey: "a",
	})
	if err != nil {
		t.Fatal(err)
	}
	to, err := providers.AddProviderProfile(ctx, domain.AddProviderProfileRequest{
		Kind: domain.ProviderOpenAICompatible, DisplayName: "B", BaseURL: openaiServer.URL, APIKey: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := bindings.CreateConversation(ctx, "p1", "Switch profile", "chat")
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{from.ID, from.ID} {
		h, err := bindings.StartRun(ctx, desktop.StartRunRequestDTO{
			ConversationID: conv.ID, Text: "q", ModelRef: "m", ModeID: "chat", ProviderID: profile,
		})
		if err != nil {
			t.Fatal(err)
		}
		for range h.Events {
		}
	}
	h, err := bindings.StartRun(ctx, desktop.StartRunRequestDTO{
		ConversationID: conv.ID, Text: "q", ModelRef: "m", ModeID: "chat", ProviderID: to.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	for range h.Events {
	}
	body := string(openaiBody)
	for _, banned := range []string{"hidden", "sig-profile", `"type":"thinking"`} {
		if strings.Contains(body, banned) {
			t.Fatalf("foreign block %q leaked after provider switch: %s", banned, body)
		}
	}
	if !strings.Contains(body, "first") || !strings.Contains(body, "second") {
		t.Fatalf("canonical text was dropped by the provider switch: %s", body)
	}
	msgs, err := store.ListMessages(ctx, conv.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		for _, part := range m.Content {
			if string(part.Raw) == thinking {
				return // canonical history is unchanged
			}
		}
	}
	t.Fatal("provider switch rewrote canonical history")
}
