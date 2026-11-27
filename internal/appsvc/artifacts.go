package appsvc

import (
	"context"
	"errors"

	"github.com/amaldevice/sasmita/internal/artifacts"
)

// Tier 2 artifact commands (TAD §14.1). They are thin pass-throughs: the edit
// protocol, limits, storage and serving rules live in internal/artifacts.

func (s *Service) artifactService() (*artifacts.Service, error) {
	if s.cfg.Artifacts == nil {
		return nil, errors.New("artifacts are not enabled for this service")
	}
	return s.cfg.Artifacts, nil
}

func (s *Service) CreateArtifact(ctx context.Context, in artifacts.CreateInput) (*artifacts.Artifact, error) {
	svc, err := s.artifactService()
	if err != nil {
		return nil, err
	}
	a, err := svc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Service) EditArtifact(ctx context.Context, id string, edits []artifacts.Edit) (*artifacts.Artifact, error) {
	svc, err := s.artifactService()
	if err != nil {
		return nil, err
	}
	a, err := svc.Edit(ctx, id, edits)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Service) RewriteArtifact(ctx context.Context, id string, in artifacts.RewriteInput) (*artifacts.Artifact, error) {
	svc, err := s.artifactService()
	if err != nil {
		return nil, err
	}
	a, err := svc.Rewrite(ctx, id, in)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Service) ListArtifacts(ctx context.Context) ([]artifacts.Artifact, error) {
	svc, err := s.artifactService()
	if err != nil {
		return nil, err
	}
	return svc.List(ctx)
}

func (s *Service) OpenArtifact(ctx context.Context, id string) (*artifacts.Artifact, error) {
	svc, err := s.artifactService()
	if err != nil {
		return nil, err
	}
	a, err := svc.Open(ctx, id)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Service) MarkArtifactFroze(ctx context.Context, id string) error {
	svc, err := s.artifactService()
	if err != nil {
		return err
	}
	return svc.MarkFroze(ctx, id)
}
