package tools

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Artifact is an opaque, stable reference to a sandboxed artifact payload.
// The reference is a URI rather than a filesystem path so chat and events never
// depend on where the payload lives on disk (TAD §13.4).
type Artifact struct {
	ID     string
	Ref    string
	Size   int
	Path   string
	Prefix string
}

// ArtifactStore writes payloads atomically under one directory.
type ArtifactStore struct {
	Dir   string
	NewID func() string
}

func NewArtifactStore(dir string) *ArtifactStore {
	return &ArtifactStore{Dir: dir, NewID: randomArtifactID}
}

func randomArtifactID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%016x", os.Getpid())
	}
	return hex.EncodeToString(b[:])
}

// Write stores content and returns its stable reference. A short plain-text
// prefix is kept alongside the artifact so chat can show a readable summary.
func (s *ArtifactStore) Write(name, content string) (Artifact, error) {
	if s.Dir == "" {
		return Artifact{}, fmt.Errorf("artifact directory is not configured")
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return Artifact{}, err
	}
	id := s.NewID()
	if id == "" {
		return Artifact{}, fmt.Errorf("artifact id is empty")
	}
	path := filepath.Join(s.Dir, id+".txt")
	tmp := filepath.Join(s.Dir, "."+id+".tmp")
	// Atomic write/rename: a crash never leaves a half-written artifact.
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return Artifact{}, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return Artifact{}, err
	}
	prefix := []rune(content)
	if len(prefix) > 160 {
		prefix = prefix[:160]
	}
	return Artifact{
		ID: id, Ref: "artifact://" + id, Size: len(content),
		Path: path, Prefix: strings.TrimSpace(string(prefix)),
	}, nil
}
