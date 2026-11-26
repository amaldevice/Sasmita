package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DataResolver lets the host confirm that a declared data reference really is
// within the per-reference cap. It is optional; tooling without a data store
// still enforces the declared size gate.
type DataResolver interface {
	Size(ref string) (int, error)
}

// Service implements the model-facing edit protocol. Every successful call
// either creates an artifact or appends one immutable version; no stored bytes
// are ever modified.
type Service struct {
	Store *Store
	Data  DataResolver
	Now   func() time.Time
}

func (s *Service) store() *Store {
	if s.Store != nil {
		return s.Store
	}
	return NewStore("")
}

func (s *Service) now() string {
	if s.Now != nil {
		return s.Now().UTC().Format(time.RFC3339Nano)
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// CreateInput is the argument shape of create_artifact.
type CreateInput struct {
	Source    string    `json:"source"`
	SourceKind SourceKind `json:"sourceKind"`
	Manifest  Manifest  `json:"manifest"`
	ProjectID string    `json:"projectId,omitempty"`
}

// RewriteInput is the argument shape of rewrite_artifact.
type RewriteInput struct {
	Source     string     `json:"source"`
	SourceKind SourceKind `json:"sourceKind"`
	Manifest   Manifest   `json:"manifest"`
}

// EditError is returned to the model when an exact-match edit cannot be
// applied. The message tells it how many matches were found so it can retry
// with a longer, unique old string.
type EditError struct {
	Index  int
	Reason string
}

func (e *EditError) Error() string {
	return fmt.Sprintf("edit %d rejected: %s", e.Index+1, e.Reason)
}

// Create stores the first immutable version.
func (s *Service) Create(ctx context.Context, in CreateInput) (Artifact, error) {
	if err := s.checkData(in.Manifest); err != nil {
		return Artifact{}, err
	}
	kind := in.SourceKind
	if kind == "" {
		kind = SourceHTML
	}
	compiled, mime, err := Compile(in.Source, kind)
	if err != nil {
		return Artifact{}, err
	}
	store := s.store()
	a := Artifact{
		ID: store.nextID(), SourceKind: kind, RuntimeVersion: RuntimeVersion,
		Manifest: in.Manifest, ProjectID: in.ProjectID, Status: StatusOK,
	}
	v := newVersion(1, in.Source, compiled, mime, kind)
	if err := store.Create(&a, v); err != nil {
		return Artifact{}, err
	}
	return store.Load(a.ID)
}

// Edit applies each exact-match replacement to the latest source in order and
// stores the result as a new immutable version. An edit with zero matches or
// more than one match rejects the whole call; no partial version is written.
func (s *Service) Edit(ctx context.Context, id string, edits []Edit) (Artifact, error) {
	if len(edits) == 0 {
		return Artifact{}, &EditError{Index: 0, Reason: "no edits supplied"}
	}
	store := s.store()
	a, err := store.Load(id)
	if err != nil {
		return Artifact{}, err
	}
	latest, err := a.Latest()
	if err != nil {
		return Artifact{}, err
	}
	source := latest.Source
	for i, edit := range edits {
		if edit.Old == "" {
			return Artifact{}, &EditError{Index: i, Reason: "old must be a non-empty exact string"}
		}
		count := strings.Count(source, edit.Old)
		if count == 0 {
			return Artifact{}, &EditError{Index: i, Reason: "old text not found; include a longer unique context"}
		}
		if count > 1 {
			return Artifact{}, &EditError{Index: i, Reason: fmt.Sprintf("old text matches %d times; provide a unique context", count)}
		}
		source = strings.Replace(source, edit.Old, edit.New, 1)
	}
	if err := s.checkData(a.Manifest); err != nil {
		return Artifact{}, err
	}
	compiled, mime, err := Compile(source, a.SourceKind)
	if err != nil {
		return Artifact{}, err
	}
	v := newVersion(len(a.Versions)+1, source, compiled, mime, a.SourceKind)
	if err := store.AppendVersion(id, v); err != nil {
		return Artifact{}, err
	}
	return store.Load(id)
}

// Rewrite replaces the whole source with a new immutable version.
func (s *Service) Rewrite(ctx context.Context, id string, in RewriteInput) (Artifact, error) {
	store := s.store()
	a, err := store.Load(id)
	if err != nil {
		return Artifact{}, err
	}
	manifest := in.Manifest
	if len(manifest.DataRefs) == 0 {
		manifest = a.Manifest
	}
	if err := s.checkData(manifest); err != nil {
		return Artifact{}, err
	}
	kind := in.SourceKind
	if kind == "" {
		kind = a.SourceKind
	}
	compiled, mime, err := Compile(in.Source, kind)
	if err != nil {
		return Artifact{}, err
	}
	next := a
	next.Manifest = manifest
	next.SourceKind = kind
	next.Status = StatusOK
	v := newVersion(len(a.Versions)+1, in.Source, compiled, mime, kind)
	if err := store.AppendVersion(id, v); err != nil {
		return Artifact{}, err
	}
	return store.Load(id)
}

func (s *Service) List(ctx context.Context) ([]Artifact, error) {
	return s.store().List()
}

func (s *Service) Open(ctx context.Context, id string) (Artifact, error) {
	return s.store().Load(id)
}

// MarkFroze records a watchdog decision. It never deletes or rewrites a
// version and never auto-runs the artifact.
func (s *Service) MarkFroze(ctx context.Context, id string) error {
	return s.store().SetStatus(id, StatusFrozeView)
}

func (s *Service) checkData(m Manifest) error {
	if err := validateManifest(m); err != nil {
		return err
	}
	if s.Data == nil {
		return nil
	}
	for _, spec := range m.DataRefs {
		size, err := s.Data.Size(spec.Ref)
		if err != nil {
			return fmt.Errorf("data ref %q: %w", spec.Ref, err)
		}
		if size > MaxDataRefBytes {
			return ErrDataRefTooBig
		}
	}
	return nil
}

func newVersion(number int, source, compiled, mime string, kind SourceKind) Version {
	sum := sha256.Sum256([]byte(source))
	return Version{
		Number: number, Source: source, Compiled: compiled, Kind: kind,
		MIMEType: mime, SHA256: hex.EncodeToString(sum[:]), Size: len(source),
	}
}

// IsEditError reports whether err is an exact-match rejection meant for the
// model rather than an infrastructure failure.
func IsEditError(err error) bool {
	var e *EditError
	return errors.As(err, &e)
}
