package appsvc_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
	"github.com/amaldevice/sasmita/internal/secrets"
)

// fakeSecrets is the swappable secrets port used by tests. Keys never reach
// the service's public results.
type fakeSecrets struct{ values map[string]string }

func newFakeSecrets() *fakeSecrets { return &fakeSecrets{values: map[string]string{}} }
func (f *fakeSecrets) Put(_ context.Context, ref, value string) error {
	f.values[ref] = value
	return nil
}
func (f *fakeSecrets) Get(_ context.Context, ref string) (string, error) {
	v, ok := f.values[ref]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return v, nil
}
func (f *fakeSecrets) Delete(_ context.Context, ref string) error {
	delete(f.values, ref)
	return nil
}

func newProviderService(t *testing.T) (*appsvc.ProviderService, *sql.DB, *fakeSecrets) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "sasmita.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	secretsStore := newFakeSecrets()
	svc := appsvc.NewProviderService(repositories.New(db), appsvc.ProviderServiceConfig{
		Secrets:    secretsStore,
		NewAdapter: adapters.New,
		NewID: func() string {
			return "profile-1"
		},
	})
	return svc, db, secretsStore
}

const rawKey = "sk-raw-secret-must-never-persist"

func TestAddProviderProfilePersistsOnlyOpaqueSecretReference(t *testing.T) {
	ctx := context.Background()
	svc, db, secretStore := newProviderService(t)

	profile, err := svc.AddProviderProfile(ctx, domain.AddProviderProfileRequest{
		Kind: domain.ProviderAnthropicCompatible, DisplayName: "Anthropic",
		BaseURL: "https://api.anthropic.com", APIKey: rawKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.SecretRef == "" || strings.Contains(profile.SecretRef, rawKey) {
		t.Fatalf("SecretRef = %q, must be an opaque reference", profile.SecretRef)
	}
	if got := secretStore.values[profile.SecretRef]; got != rawKey {
		t.Fatalf("secret store value = %q, want raw key held only by the store", got)
	}

	// No column and no row in the SQLite file may contain the raw key.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM provider_profiles WHERE secret_ref LIKE '%sk-%'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("raw key was persisted in provider_profiles (rows = %d)", count)
	}
	var blob string
	if err := db.QueryRow(`SELECT group_concat(name || '=' || value) FROM (SELECT name, sql AS value FROM sqlite_master)`).Scan(&blob); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(blob, rawKey) {
		t.Fatal("raw key appeared in the SQLite schema")
	}

	listed, err := svc.ListProviderProfiles(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListProviderProfiles = %+v (err %v)", listed, err)
	}
	if strings.Contains(fmt.Sprint(listed), rawKey) {
		t.Fatalf("profile DTO leaked the key: %+v", listed)
	}
}

func TestListModelsShowsUnknownCapabilitiesInsteadOfGuessing(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Key"); got != rawKey {
			t.Errorf("x-api-key = %q", got)
		}
		fmt.Fprint(w, `{"data":[{"id":"model-a"},{"id":"model-b","context_window":200000,"supports_tools":true,"supports_vision":false}]}`)
	}))
	defer srv.Close()

	svc, _, _ := newProviderService(t)
	profile, err := svc.AddProviderProfile(ctx, domain.AddProviderProfileRequest{
		Kind: domain.ProviderAnthropicCompatible, DisplayName: "Anthropic Fake",
		BaseURL: srv.URL, APIKey: rawKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	models, err := svc.ListModels(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %+v", models)
	}
	if models[0].ID != "model-a" {
		t.Fatalf("model[0].ID = %q", models[0].ID)
	}
	if models[0].Capabilities.Tools != model.CapabilityUnknown ||
		models[0].Capabilities.Vision != model.CapabilityUnknown ||
		models[0].Capabilities.ContextWindow != nil {
		t.Fatalf("model-a capabilities were guessed: %+v", models[0].Capabilities)
	}
	if models[1].Capabilities.Tools != model.CapabilitySupported ||
		models[1].Capabilities.Vision != model.CapabilityUnsupported ||
		models[1].Capabilities.ContextWindow == nil || *models[1].Capabilities.ContextWindow != 200000 {
		t.Fatalf("model-b capabilities = %+v", models[1].Capabilities)
	}
}

func TestTestConnectionPlainLanguageOutcomes(t *testing.T) {
	ctx := context.Background()
	success := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"m"}]}`)
	}))
	defer success.Close()
	badKey := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`)
	}))
	defer badKey.Close()
	unreachable := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := unreachable.URL
	unreachable.Close()

	tests := []struct {
		name, baseURL, want string
		wantOK              bool
	}{
		{"success", success.URL, "Connected", true},
		{"bad key", badKey.URL, "API key was rejected", false},
		{"unreachable", deadURL, "Could not reach", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := newProviderService(t)
			profile, err := svc.AddProviderProfile(ctx, domain.AddProviderProfileRequest{
				Kind: domain.ProviderAnthropicCompatible, DisplayName: "Anthropic Fake",
				BaseURL: tc.baseURL, APIKey: rawKey,
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := svc.TestConnection(ctx, profile.ID)
			if err != nil {
				t.Fatal(err)
			}
			if result.OK != tc.wantOK || !strings.Contains(result.Message, tc.want) {
				t.Fatalf("result = %+v, want ok=%v and message containing %q", result, tc.wantOK, tc.want)
			}
			if strings.Contains(result.Message, rawKey) {
				t.Fatalf("result leaked the key: %+v", result)
			}
		})
	}
}

func TestProviderServiceValidatesInputAndDeletesSecret(t *testing.T) {
	ctx := context.Background()
	svc, _, secretStore := newProviderService(t)
	if _, err := svc.AddProviderProfile(ctx, domain.AddProviderProfileRequest{
		Kind: "nonsense", DisplayName: "x", BaseURL: "https://x.test", APIKey: "k",
	}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("invalid kind error = %v", err)
	}
	if _, err := svc.AddProviderProfile(ctx, domain.AddProviderProfileRequest{
		Kind: domain.ProviderOpenAICompatible, DisplayName: "x", BaseURL: "not a url", APIKey: "k",
	}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("invalid base URL error = %v", err)
	}

	profile, err := svc.AddProviderProfile(ctx, domain.AddProviderProfileRequest{
		Kind: domain.ProviderOpenAICompatible, DisplayName: "OpenAI Fake",
		BaseURL: "https://api.openai.com", APIKey: rawKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteProviderProfile(ctx, profile.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := secretStore.values[profile.SecretRef]; ok {
		t.Fatal("secret remained in the store after profile deletion")
	}
}
