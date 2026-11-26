package appsvc

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/secrets"
)

// AdapterFactory builds a provider adapter for a profile. It is injectable so
// tests can supply a fake wire implementation.
type AdapterFactory func(kind, baseURL, apiKey, providerID string) (model.ModelProvider, error)

type ProviderServiceConfig struct {
	Secrets    secrets.Store
	NewAdapter AdapterFactory
	Now        func() time.Time
	NewID      func() string
}

// ProviderService owns the provider-profile use cases from TAD §14.1. It never
// holds a raw credential: keys are read from the secret port at request time.
type ProviderService struct {
	store *repositories.Store
	cfg   ProviderServiceConfig
}

func NewProviderService(store *repositories.Store, cfg ProviderServiceConfig) *ProviderService {
	if cfg.Secrets == nil {
		cfg.Secrets = secrets.NewKeychain()
	}
	if cfg.NewAdapter == nil {
		cfg.NewAdapter = adapters.New
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.NewID == nil {
		cfg.NewID = randomID
	}
	return &ProviderService{store: store, cfg: cfg}
}

func validBaseURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func validateProfile(kind domain.ProviderKind, displayName, baseURL string) error {
	if !kind.Valid() || strings.TrimSpace(displayName) == "" || !validBaseURL(baseURL) {
		return domain.ErrInvalidArgument
	}
	return nil
}

func (s *ProviderService) AddProviderProfile(ctx context.Context, req domain.AddProviderProfileRequest) (*domain.ProviderProfile, error) {
	if err := validateProfile(req.Kind, req.DisplayName, req.BaseURL); err != nil {
		return nil, err
	}
	id := s.cfg.NewID()
	ref := "sasmita-provider-" + id
	if err := s.cfg.Secrets.Put(ctx, ref, req.APIKey); err != nil {
		return nil, err
	}
	now := s.cfg.Now()
	p := &domain.ProviderProfile{
		ID: id, Kind: req.Kind, DisplayName: req.DisplayName,
		BaseURL: strings.TrimSpace(req.BaseURL), SecretRef: ref,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.CreateProviderProfile(ctx, p); err != nil {
		s.cfg.Secrets.Delete(ctx, ref)
		return nil, err
	}
	return p, nil
}

func (s *ProviderService) ListProviderProfiles(ctx context.Context) ([]domain.ProviderProfile, error) {
	return s.store.ListProviderProfiles(ctx)
}

func (s *ProviderService) UpdateProviderProfile(ctx context.Context, req domain.UpdateProviderProfileRequest) (*domain.ProviderProfile, error) {
	p, err := s.store.GetProviderProfile(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	kind := req.Kind
	if kind == "" {
		kind = p.Kind
	}
	if err := validateProfile(kind, req.DisplayName, req.BaseURL); err != nil {
		return nil, err
	}
	if req.APIKey != "" {
		if err := s.cfg.Secrets.Put(ctx, p.SecretRef, req.APIKey); err != nil {
			return nil, err
		}
	}
	p.Kind, p.DisplayName = kind, req.DisplayName
	p.BaseURL, p.UpdatedAt = strings.TrimSpace(req.BaseURL), s.cfg.Now()
	if err := s.store.UpdateProviderProfile(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *ProviderService) DeleteProviderProfile(ctx context.Context, id string) error {
	p, err := s.store.GetProviderProfile(ctx, id)
	if err != nil {
		return err
	}
	if err := s.store.DeleteProviderProfile(ctx, id); err != nil {
		return err
	}
	return s.cfg.Secrets.Delete(ctx, p.SecretRef)
}

func (s *ProviderService) adapterFor(ctx context.Context, p *domain.ProviderProfile) (model.ModelProvider, error) {
	key, err := s.cfg.Secrets.Get(ctx, p.SecretRef)
	if err != nil {
		return nil, fmt.Errorf("read credential for %s: %w", p.DisplayName, err)
	}
	return s.cfg.NewAdapter(string(p.Kind), p.BaseURL, key, "profile-"+p.ID)
}

// ListModels returns provider-reported capabilities. Values the provider did
// not report are returned as unknown, never guessed (TAD §9.1).
func (s *ProviderService) ListModels(ctx context.Context, profileID string) ([]model.ModelInfo, error) {
	p, err := s.store.GetProviderProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	adapter, err := s.adapterFor(ctx, p)
	if err != nil {
		return nil, err
	}
	lister, ok := adapter.(model.ModelLister)
	if !ok {
		return nil, fmt.Errorf("provider %s cannot list models", p.Kind)
	}
	return lister.ListModels(ctx)
}

func (s *ProviderService) probe(ctx context.Context, p *domain.ProviderProfile) error {
	adapter, err := s.adapterFor(ctx, p)
	if err != nil {
		return err
	}
	lister, ok := adapter.(model.ModelLister)
	if !ok {
		return fmt.Errorf("provider %s cannot list models", p.Kind)
	}
	_, err = lister.ListModels(ctx)
	return err
}

// TestConnection turns provider errors into plain language for the settings UI.
// Credentials never appear in the result.
func (s *ProviderService) TestConnection(ctx context.Context, profileID string) (domain.ConnectionResult, error) {
	p, err := s.store.GetProviderProfile(ctx, profileID)
	if err != nil {
		return domain.ConnectionResult{}, err
	}
	err = s.probe(ctx, p)
	switch {
	case err == nil:
		return domain.ConnectionResult{OK: true, Message: fmt.Sprintf("Connected to %s.", p.DisplayName)}, nil
	case errors.Is(err, model.ErrUnauthorized):
		return domain.ConnectionResult{OK: false, Message: fmt.Sprintf("The API key was rejected by %s. Check the key and try again.", p.DisplayName)}, nil
	case errors.Is(err, model.ErrUnreachable):
		host := p.BaseURL
		if u, parseErr := url.Parse(p.BaseURL); parseErr == nil && u.Host != "" {
			host = u.Host
		}
		return domain.ConnectionResult{OK: false, Message: fmt.Sprintf("Could not reach %s. Check the base URL and your network.", host)}, nil
	default:
		return domain.ConnectionResult{OK: false, Message: fmt.Sprintf("%s returned an error: %s", p.DisplayName, strings.TrimSpace(err.Error()))}, nil
	}
}
