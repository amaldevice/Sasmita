package domain

import "time"

// ProviderKind selects the wire format an adapter speaks. Each profile carries
// its own base URL; credentials live in the OS keychain (TAD §13.3, §15.6).
type ProviderKind string

const (
	ProviderOpenAICompatible    ProviderKind = "openai-compatible"
	ProviderAnthropicCompatible ProviderKind = "anthropic-compatible"
)

func (k ProviderKind) Valid() bool {
	return k == ProviderOpenAICompatible || k == ProviderAnthropicCompatible
}

// ProviderProfile is safe to persist and display: SecretRef is an opaque
// keychain reference, never a raw API key.
type ProviderProfile struct {
	ID          string
	Kind        ProviderKind
	DisplayName string
	BaseURL     string
	SecretRef   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AddProviderProfileRequest is the typed command for adding a profile. The key
// is accepted here only to be handed straight to the secret store.
type AddProviderProfileRequest struct {
	Kind        ProviderKind `json:"kind"`
	DisplayName string       `json:"displayName"`
	BaseURL     string       `json:"baseUrl"`
	APIKey      string       `json:"apiKey"`
}

// UpdateProviderProfileRequest carries an empty APIKey when the credential is
// unchanged.
type UpdateProviderProfileRequest struct {
	ID          string       `json:"id"`
	Kind        ProviderKind `json:"kind"`
	DisplayName string       `json:"displayName"`
	BaseURL     string       `json:"baseUrl"`
	APIKey      string       `json:"apiKey"`
}

// ConnectionResult is a plain-language outcome for Test connection.
type ConnectionResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}
