package desktop_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/desktop"
	"github.com/amaldevice/sasmita/internal/onboarding"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

// Onboarding data crosses the same typed binding surface as the rest of the
// shell. The probe is injectable so the binding can be tested without a real
// model listening on a local port.
func TestOnboardingBindingsExposeCatalogAndDetectedEndpoints(t *testing.T) {
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
	providerSvc := appsvc.NewProviderService(store, appsvc.ProviderServiceConfig{
		Secrets: &fakeSecrets{values: map[string]string{}},
	})
	bindings := desktop.NewServiceWithProviders(
		appsvc.New(store, appsvc.Config{Provider: stubProvider{}}), providerSvc,
	)
	bindings.SetLocalProbe(func(_ context.Context, endpoint onboarding.LocalEndpoint) error {
		if endpoint.Kind == onboarding.OllamaKind {
			return nil
		}
		return errors.New("offline")
	})
	var surface desktop.Bindings = bindings

	prompts := surface.ListExamplePrompts(ctx)
	if len(prompts) != 4 || prompts[0].VisualHint == "" {
		t.Fatalf("ListExamplePrompts = %+v", prompts)
	}

	endpoints, err := surface.DetectLocalEndpoints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(endpoints) != 1 || endpoints[0].Kind != onboarding.OllamaKind {
		t.Fatalf("DetectLocalEndpoints = %+v", endpoints)
	}
}
