// Package artifacts owns Tier 2 sandboxed artifacts (ADR-0004). Model-written
// HTML/JSX is stored as immutable versions, transpiled with esbuild, and served
// read-only through artifact:// with a strict CSP. No code in this package runs
// model output: it only stores, transforms, and serves bytes.
package artifacts

import (
	"errors"
	"fmt"
	"strings"
)

const (
	// MaxSourceBytes matches artifacts_runtime.max_source_bytes in the TAD.
	MaxSourceBytes = 512 * 1024
	// MaxDataRefBytes caps each declared data reference behind getData(ref).
	MaxDataRefBytes = 5 * 1024 * 1024
	// MaxStateBytes caps saveState/loadState payloads on the bridge.
	MaxStateBytes = 32 * 1024
	// RuntimeVersion is the pinned artifact runtime served by the import map.
	RuntimeVersion = "artifact-runtime/1"
	// Scheme is the custom MyGo scheme artifacts are served from.
	Scheme = "artifact"
)

// SourceKind selects how a stored source is turned into runnable bytes.
type SourceKind string

const (
	SourceHTML SourceKind = "html"
	SourceJSX  SourceKind = "jsx"
)

// Status is the lifecycle state of an artifact. "froze_view" is set by the Go
// watchdog when an artifact blocks the shared WebContent main thread; the host
// never auto-runs an artifact in that state.
type Status string

const (
	StatusOK        Status = "ok"
	StatusFrozeView Status = "froze_view"
)

// DataRefSpec declares one data reference an artifact will request through the
// bridge. Only the declared reference and its declared size are visible to the
// host; the model never passes arbitrary filesystem paths to the renderer.
type DataRefSpec struct {
	Ref       string `json:"ref"`
	SizeBytes int    `json:"sizeBytes"`
}

// Manifest is the persisted manifest_json field (TAD §13.3).
type Manifest struct {
	DataRefs map[string]DataRefSpec `json:"dataRefs,omitempty"`
}

// Edit is one exact-match replacement. A replacement is applied only when the
// old text occurs exactly once in the current source; 0 or >1 matches is an
// error that is returned to the model so it can correct its edit.
type Edit struct {
	Old string `json:"old"`
	New string `json:"new"`
}

// Version is immutable once written. Source is the model text, Compiled is the
// bytes actually served (esbuild output for JSX, the HTML document for HTML).
type Version struct {
	Number    int       `json:"version"`
	Ref       string    `json:"ref"`
	Source    string    `json:"-"`
	Compiled  string    `json:"-"`
	Kind      SourceKind `json:"sourceKind"`
	MIMEType  string    `json:"mimeType"`
	SHA256    string    `json:"sha256"`
	Size      int       `json:"sizeBytes"`
	CreatedAt string    `json:"createdAt"`
}

// Artifact is the stable identity of an artifact. Every change appends a new
// Version; older versions are never modified or removed.
type Artifact struct {
	ID             string    `json:"id"`
	Ref            string    `json:"ref"`
	ProjectID      string    `json:"projectId,omitempty"`
	URI            string    `json:"uri"`
	SourceKind     SourceKind `json:"sourceKind"`
	RuntimeVersion string    `json:"runtimeVersion"`
	Manifest       Manifest  `json:"manifest"`
	Status         Status    `json:"status"`
	CreatedAt      string    `json:"createdAt"`
	UpdatedAt      string    `json:"updatedAt"`
	Versions       []Version `json:"versions"`
}

// Latest returns the most recent version, or an error for an empty artifact.
func (a Artifact) Latest() (Version, error) {
	if len(a.Versions) == 0 {
		return Version{}, errors.New("artifact has no versions")
	}
	return a.Versions[len(a.Versions)-1], nil
}

var (
	ErrEmptySource   = errors.New("artifact source is empty")
	ErrSourceTooBig  = fmt.Errorf("artifact source exceeds %d bytes", MaxSourceBytes)
	ErrDataRefTooBig = fmt.Errorf("artifact data reference exceeds %d bytes", MaxDataRefBytes)
	ErrNotFound      = errors.New("artifact not found")
	ErrInvalidID     = errors.New("invalid artifact id")
)

func validID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

// validateSource applies the size gate before any bytes are stored or run.
func validateSource(source string) error {
	if strings.TrimSpace(source) == "" {
		return ErrEmptySource
	}
	if len(source) > MaxSourceBytes {
		return ErrSourceTooBig
	}
	return nil
}

func validateManifest(m Manifest) error {
	for name, spec := range m.DataRefs {
		if spec.Ref == "" {
			return fmt.Errorf("data ref %q has no reference", name)
		}
		if spec.SizeBytes < 0 || spec.SizeBytes > MaxDataRefBytes {
			return ErrDataRefTooBig
		}
	}
	return nil
}
