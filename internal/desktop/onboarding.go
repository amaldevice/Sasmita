package desktop

import (
	"context"

	"github.com/amaldevice/sasmita/internal/onboarding"
)

// The catalog and detection results are plain data; reusing the onboarding
// types keeps the JSON shape identical on both sides of IPC.
type ExamplePromptDTO = onboarding.ExamplePrompt
type LocalEndpointDTO = onboarding.LocalEndpoint

// SetLocalProbe replaces the endpoint probe. The shell uses the real HTTP
// probe by default; tests inject one so no local server is required.
func (s *Service) SetLocalProbe(probe onboarding.Probe) {
	s.localProbe = probe
}

func (s *Service) ListExamplePrompts(context.Context) []ExamplePromptDTO {
	return onboarding.ListExamplePrompts()
}

func (s *Service) DetectLocalEndpoints(ctx context.Context) ([]LocalEndpointDTO, error) {
	probe := s.localProbe
	if probe == nil {
		probe = onboarding.HTTPProbe(nil)
	}
	return onboarding.DetectLocalEndpoints(ctx, probe), nil
}
