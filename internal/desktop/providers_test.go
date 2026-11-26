package desktop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/desktop"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

type fakeSecrets struct{ values map[string]string }

func (f *fakeSecrets) Put(_ context.Context, ref, value string) error {
	f.values[ref] = value
	return nil
}
func (f *fakeSecrets) Get(_ context.Context, ref string) (string, error) {
	return f.values[ref], nil
}
func (f *fakeSecrets) Delete(_ context.Context, ref string) error {
	delete(f.values, ref)
	return nil
}

// The typed IPC surface must expose connection commands and must never return
// a credential or a keychain reference to the frontend.
func TestBindingsConnectionCommandsDoNotLeakSecrets(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "sasmita.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	secretStore := &fakeSecrets{values: map[string]string{}}
	providerSvc := appsvc.NewProviderService(store, appsvc.ProviderServiceConfig{
		Secrets: secretStore, NewAdapter: adapters.New,
		NewID: func() string { return "profile-1" },
	})
	bindings := desktop.NewServiceWithProviders(
		appsvc.New(store, appsvc.Config{Provider: stubProvider{}}), providerSvc,
	)
	var surface desktop.Bindings = bindings

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"model-a"}]}`)
	}))
	defer srv.Close()

	const key = "sk-never-cross-ipc"
	created, err := surface.AddProviderProfile(ctx, desktop.AddProviderProfileRequestDTO{
		Kind: string(domain.ProviderAnthropicCompatible), DisplayName: "Anthropic",
		BaseURL: srv.URL, APIKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(created)
	if err != nil {
		t.Fatal(err)
	}
	var secretRef string
	for ref, value := range secretStore.values {
		if value == key {
			secretRef = ref
		}
	}
	if secretRef == "" {
		t.Fatal("key was not stored behind the secret port")
	}
	if strings.Contains(string(raw), key) || strings.Contains(string(raw), secretRef) {
		t.Fatalf("profile DTO leaked secret material: %s", raw)
	}

	all, err := surface.ListProviderProfiles(ctx)
	if err != nil || len(all) != 1 || all[0].ID != created.ID {
		t.Fatalf("ListProviderProfiles = %+v (err %v)", all, err)
	}

	models, err := surface.ListModels(ctx, created.ID)
	if err != nil || len(models) != 1 || models[0].Capabilities.Tools != string(model.CapabilityUnknown) {
		t.Fatalf("ListModels = %+v (err %v)", models, err)
	}
	rawModels, _ := json.Marshal(models)
	if strings.Contains(string(rawModels), key) {
		t.Fatalf("model DTO leaked the key: %s", rawModels)
	}

	result, err := surface.TestConnection(ctx, created.ID)
	if err != nil || !result.OK || !strings.Contains(result.Message, "Connected") {
		t.Fatalf("TestConnection = %+v (err %v)", result, err)
	}
	if strings.Contains(result.Message, key) {
		t.Fatalf("connection result leaked the key: %+v", result)
	}

	if err := surface.DeleteProviderProfile(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := surface.ListProviderProfiles(ctx); err != nil {
		t.Fatal(err)
	}
}
