// Package adapters is the provider registry. Application services build a
// concrete adapter here without importing any provider SDK (ADR-0002).
package adapters

import (
	"errors"

	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters/anthropic"
	"github.com/amaldevice/sasmita/internal/model/adapters/openai"
)

var ErrUnknownProvider = errors.New("unknown provider kind")

// New builds the adapter for a profile kind. Credentials are passed in only at
// request time from the secret store; they are never persisted with the profile.
func New(kind, baseURL, apiKey, providerID string) (model.ModelProvider, error) {
	switch kind {
	case model.ProviderOpenAICompatible:
		return openai.Adapter{BaseURL: baseURL, APIKey: apiKey, ProviderID: providerID}, nil
	case model.ProviderAnthropicCompatible:
		return anthropic.Adapter{BaseURL: baseURL, APIKey: apiKey, ProviderID: providerID}, nil
	default:
		return nil, ErrUnknownProvider
	}
}
