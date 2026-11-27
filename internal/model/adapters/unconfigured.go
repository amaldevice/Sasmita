package adapters

import (
	"context"
	"errors"

	"github.com/amaldevice/sasmita/internal/model"
)

// ErrNoProviderConfigured is returned when a run starts before the user has
// added a provider profile. The shell must still open so onboarding can run.
var ErrNoProviderConfigured = errors.New("no provider configured: add a provider in Settings or onboarding")

// Unconfigured is the startup provider when no shell default is set. It does
// no network I/O and fails closed with a plain-language error if a run is
// attempted before a profile is configured in the UI.
type Unconfigured struct{}

var _ model.ModelProvider = Unconfigured{}

func (Unconfigured) Name() string { return "unconfigured" }

func (Unconfigured) Stream(_ context.Context, _ model.StreamRequest, _ func(model.StreamEvent)) error {
	return ErrNoProviderConfigured
}
