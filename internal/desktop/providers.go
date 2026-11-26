package desktop

import (
	"context"
	"errors"
	"time"

	"github.com/amaldevice/sasmita/internal/domain"
)

// ProviderProfileDTO deliberately has no credential or secret-reference field:
// neither may ever cross IPC (TAD §15.6).
type ProviderProfileDTO struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	DisplayName string    `json:"displayName"`
	BaseURL     string    `json:"baseUrl"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type AddProviderProfileRequestDTO struct {
	Kind        string `json:"kind"`
	DisplayName string `json:"displayName"`
	BaseURL     string `json:"baseUrl"`
	APIKey      string `json:"apiKey"`
}

type UpdateProviderProfileRequestDTO struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	DisplayName string `json:"displayName"`
	BaseURL     string `json:"baseUrl"`
	APIKey      string `json:"apiKey"`
}

type CapabilitiesDTO struct {
	Tools         string `json:"tools"`
	Vision        string `json:"vision"`
	ContextWindow *int64 `json:"contextWindow"`
}

type ModelInfoDTO struct {
	ID           string          `json:"id"`
	Capabilities CapabilitiesDTO `json:"capabilities"`
}

type ConnectionResultDTO struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

var errProvidersUnavailable = errors.New("provider profiles are not configured")

func (s *Service) AddProviderProfile(ctx context.Context, req AddProviderProfileRequestDTO) (ProviderProfileDTO, error) {
	if s.providers == nil {
		return ProviderProfileDTO{}, errProvidersUnavailable
	}
	p, err := s.providers.AddProviderProfile(ctx, domain.AddProviderProfileRequest{
		Kind: domain.ProviderKind(req.Kind), DisplayName: req.DisplayName,
		BaseURL: req.BaseURL, APIKey: req.APIKey,
	})
	if err != nil {
		return ProviderProfileDTO{}, err
	}
	return providerProfileDTO(*p), nil
}

func (s *Service) ListProviderProfiles(ctx context.Context) ([]ProviderProfileDTO, error) {
	if s.providers == nil {
		return nil, errProvidersUnavailable
	}
	profiles, err := s.providers.ListProviderProfiles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderProfileDTO, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, providerProfileDTO(p))
	}
	return out, nil
}

func (s *Service) UpdateProviderProfile(ctx context.Context, req UpdateProviderProfileRequestDTO) (ProviderProfileDTO, error) {
	if s.providers == nil {
		return ProviderProfileDTO{}, errProvidersUnavailable
	}
	p, err := s.providers.UpdateProviderProfile(ctx, domain.UpdateProviderProfileRequest{
		ID: req.ID, Kind: domain.ProviderKind(req.Kind), DisplayName: req.DisplayName,
		BaseURL: req.BaseURL, APIKey: req.APIKey,
	})
	if err != nil {
		return ProviderProfileDTO{}, err
	}
	return providerProfileDTO(*p), nil
}

func (s *Service) DeleteProviderProfile(ctx context.Context, id string) error {
	if s.providers == nil {
		return errProvidersUnavailable
	}
	return s.providers.DeleteProviderProfile(ctx, id)
}

func (s *Service) ListModels(ctx context.Context, profileID string) ([]ModelInfoDTO, error) {
	if s.providers == nil {
		return nil, errProvidersUnavailable
	}
	models, err := s.providers.ListModels(ctx, profileID)
	if err != nil {
		return nil, err
	}
	out := make([]ModelInfoDTO, 0, len(models))
	for _, m := range models {
		out = append(out, ModelInfoDTO{
			ID: m.ID,
			Capabilities: CapabilitiesDTO{
				Tools: string(m.Capabilities.Tools), Vision: string(m.Capabilities.Vision),
				ContextWindow: m.Capabilities.ContextWindow,
			},
		})
	}
	return out, nil
}

func (s *Service) TestConnection(ctx context.Context, profileID string) (ConnectionResultDTO, error) {
	if s.providers == nil {
		return ConnectionResultDTO{}, errProvidersUnavailable
	}
	result, err := s.providers.TestConnection(ctx, profileID)
	if err != nil {
		return ConnectionResultDTO{}, err
	}
	return ConnectionResultDTO{OK: result.OK, Message: result.Message}, nil
}

func providerProfileDTO(p domain.ProviderProfile) ProviderProfileDTO {
	return ProviderProfileDTO{
		ID: p.ID, Kind: string(p.Kind), DisplayName: p.DisplayName,
		BaseURL: p.BaseURL, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}
