package model

import "context"

// Provider kinds select the wire format of an adapter. Each profile carries
// its own base URL and its own credential.
const (
	ProviderOpenAICompatible    = "openai-compatible"
	ProviderAnthropicCompatible = "anthropic-compatible"
)

// CapabilityState keeps "unknown" distinct from "not supported". Capabilities
// are never inferred from a model's marketing name (TAD §9.1).
type CapabilityState string

const (
	CapabilityUnknown     CapabilityState = "unknown"
	CapabilitySupported   CapabilityState = "supported"
	CapabilityUnsupported CapabilityState = "unsupported"
)

// ModelCapabilities is the capability profile shown in the model list. A nil
// ContextWindow means the provider did not report one.
type ModelCapabilities struct {
	Tools         CapabilityState `json:"tools"`
	Vision        CapabilityState `json:"vision"`
	ContextWindow *int64          `json:"contextWindow"`
}

type ModelInfo struct {
	ID           string            `json:"id"`
	Capabilities ModelCapabilities `json:"capabilities"`
}

// ModelLister is an optional provider capability used by the profile UI.
type ModelLister interface {
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

func BoolCapability(v *bool) CapabilityState {
	if v == nil {
		return CapabilityUnknown
	}
	if *v {
		return CapabilitySupported
	}
	return CapabilityUnsupported
}
