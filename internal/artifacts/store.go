package artifacts

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// metaFile is the on-disk mirror of an artifact's metadata. Payload bytes live
// in sibling files so a checkpoint never rewrites source after it is stored.
type metaFile struct {
	ID             string    `json:"id"`
	Ref            string    `json:"ref"`
	URI            string    `json:"uri"`
	ProjectID      string    `json:"projectId,omitempty"`
	SourceKind     SourceKind `json:"sourceKind"`
	RuntimeVersion string    `json:"runtimeVersion"`
	Manifest       Manifest  `json:"manifest"`
	Status         Status    `json:"status"`
	CreatedAt      string    `json:"createdAt"`
	UpdatedAt      string    `json:"updatedAt"`
	Versions       []Version `json:"versions"`
}

// Store persists immutable artifact versions. The filesystem is authoritative
// for payloads and metadata; an optional database mirrors the metadata so the
// artifacts library can list everything without scanning the disk.
type Store struct {
	Dir   string
	NewID func() string
	DB    *sql.DB
}

func NewStore(dir string) *Store {
	return &Store{Dir: dir, NewID: randomID}
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func (s *Store) nextID() string {
	if s.NewID != nil {
		return s.NewID()
	}
	return randomID()
}

func (s *Store) dir(id string) string { return filepath.Join(s.Dir, id) }
func (s *Store) metaPath(id string) string { return filepath.Join(s.dir(id), "meta.json") }

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Create writes the first version of a new artifact.
func (s *Store) Create(a *Artifact, v Version) error {
	if s.Dir == "" {
		return errors.New("artifact directory is not configured")
	}
	if !validID(a.ID) {
		return ErrInvalidID
	}
	if v.Number != 1 {
		return fmt.Errorf("first version must be 1, got %d", v.Number)
	}
	if _, err := os.Stat(s.metaPath(a.ID)); err == nil {
		return fmt.Errorf("artifact %s already exists", a.ID)
	}
	return s.writeVersion(a, v, true)
}

// AppendVersion adds a new immutable version to an existing artifact.
func (s *Store) AppendVersion(id string, v Version) error {
	a, err := s.Load(id)
	if err != nil {
		return err
	}
	if v.Number != len(a.Versions)+1 {
		return fmt.Errorf("expected version %d, got %d", len(a.Versions)+1, v.Number)
	}
	return s.writeVersion(&a, v, false)
}

func (s *Store) writeVersion(a *Artifact, v Version, isNew bool) error {
	base := s.dir(a.ID)
	if err := os.MkdirAll(base, 0o700); err != nil {
		return err
	}
	name := "v" + strconv.Itoa(v.Number)
	if err := atomicWrite(filepath.Join(base, name+".source"), []byte(v.Source)); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(base, name+".compiled"), []byte(v.Compiled)); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if isNew {
		a.Ref = Scheme + "://" + a.ID
		a.URI = a.Ref + "/"
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	if a.Status == "" {
		a.Status = StatusOK
	}
	if a.RuntimeVersion == "" {
		a.RuntimeVersion = RuntimeVersion
	}
	stored := Version{
		Number: v.Number, Ref: a.Ref + "#v" + strconv.Itoa(v.Number),
		Kind: v.Kind, MIMEType: v.MIMEType, SHA256: v.SHA256,
		Size: v.Size, CreatedAt: now,
	}
	a.Versions = append(a.Versions, stored)
	meta, err := json.MarshalIndent(metaFrom(a), "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(s.metaPath(a.ID), meta); err != nil {
		return err
	}
	return s.mirror(a)
}

func metaFrom(a *Artifact) metaFile {
	return metaFile{
		ID: a.ID, Ref: a.Ref, URI: a.URI, ProjectID: a.ProjectID, SourceKind: a.SourceKind,
		RuntimeVersion: a.RuntimeVersion, Manifest: a.Manifest, Status: a.Status,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt, Versions: a.Versions,
	}
}

func artifactFrom(m metaFile) Artifact {
	return Artifact{
		ID: m.ID, Ref: m.Ref, ProjectID: m.ProjectID, URI: m.URI,
		SourceKind: m.SourceKind, RuntimeVersion: m.RuntimeVersion,
		Manifest: m.Manifest, Status: m.Status, Versions: m.Versions,
	}
}

// Load reads metadata and reattaches the immutable payload bytes for every
// version. It never mutates what is on disk.
func (s *Store) Load(id string) (Artifact, error) {
	if !validID(id) {
		return Artifact{}, ErrInvalidID
	}
	raw, err := os.ReadFile(s.metaPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Artifact{}, ErrNotFound
		}
		return Artifact{}, err
	}
	var m metaFile
	if err := json.Unmarshal(raw, &m); err != nil {
		return Artifact{}, fmt.Errorf("artifact %s metadata: %w", id, err)
	}
	if m.Ref == "" {
		m.Ref = Scheme + "://" + m.ID
	}
	if m.URI == "" {
		m.URI = m.Ref + "/"
	}
	a := artifactFrom(m)
	for i := range a.Versions {
		name := "v" + strconv.Itoa(a.Versions[i].Number)
		source, err := os.ReadFile(filepath.Join(s.dir(id), name+".source"))
		if err != nil {
			return Artifact{}, fmt.Errorf("artifact %s version %d source: %w", id, a.Versions[i].Number, err)
		}
		compiled, err := os.ReadFile(filepath.Join(s.dir(id), name+".compiled"))
		if err != nil {
			return Artifact{}, fmt.Errorf("artifact %s version %d compiled: %w", id, a.Versions[i].Number, err)
		}
		a.Versions[i].Source = string(source)
		a.Versions[i].Compiled = string(compiled)
	}
	return a, nil
}

// List returns every artifact. The database is used when present so the
// library view has one authoritative index; otherwise the directory is scanned.
func (s *Store) List() ([]Artifact, error) {
	if s.DB != nil {
		return s.listDB()
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Artifact, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || !validID(e.Name()) {
			continue
		}
		a, err := s.Load(e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

// SetStatus records a watchdog decision without touching any version.
func (s *Store) SetStatus(id string, status Status) error {
	if status != StatusOK && status != StatusFrozeView {
		return fmt.Errorf("unknown artifact status %q", status)
	}
	a, err := s.Load(id)
	if err != nil {
		return err
	}
	a.Status = status
	meta, err := json.MarshalIndent(metaFrom(&a), "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(s.metaPath(id), meta); err != nil {
		return err
	}
	return s.mirror(&a)
}

// VersionSource returns the source of one specific immutable version.
func (s *Store) VersionSource(id string, version int) (Version, error) {
	a, err := s.Load(id)
	if err != nil {
		return Version{}, err
	}
	for _, v := range a.Versions {
		if v.Number == version {
			return v, nil
		}
	}
	return Version{}, fmt.Errorf("artifact %s has no version %d", id, version)
}

func (s *Store) mirror(a *Artifact) error {
	if s.DB == nil {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	manifest, err := json.Marshal(a.Manifest)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO artifacts(id, project_id, kind, uri, mime_type, size_bytes, sha256,
			runtime_version, source_kind, manifest_json, status, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			uri=excluded.uri, size_bytes=excluded.size_bytes, sha256=excluded.sha256,
			manifest_json=excluded.manifest_json, status=excluded.status, updated_at=excluded.updated_at`,
		a.ID, a.ProjectID, "sandboxed", a.URI, a.SourceKind, 0, "",
		a.RuntimeVersion, string(a.SourceKind), string(manifest), string(a.Status), a.CreatedAt, a.UpdatedAt); err != nil {
		return fmt.Errorf("mirror artifact: %w", err)
	}
	for _, v := range a.Versions {
		if _, err := tx.Exec(`
			INSERT INTO artifact_versions(id, artifact_id, version, uri, source_ref, compiled_ref,
				sha256, size_bytes, created_at)
			VALUES(?,?,?,?,?,?,?,?,?)
			ON CONFLICT(artifact_id, version) DO UPDATE SET
				sha256=excluded.sha256, size_bytes=excluded.size_bytes`,
			a.ID+"-v"+strconv.Itoa(v.Number), a.ID, v.Number, v.Ref,
			Scheme+"://"+a.ID+"/v"+strconv.Itoa(v.Number)+".source",
			Scheme+"://"+a.ID+"/v"+strconv.Itoa(v.Number)+".compiled",
			v.SHA256, v.Size, v.CreatedAt); err != nil {
			return fmt.Errorf("mirror version: %w", err)
		}
	}
	return tx.Commit()
}

func (s *Store) listDB() ([]Artifact, error) {
	rows, err := s.DB.Query(`SELECT id FROM artifacts ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Artifact, 0, len(ids))
	for _, id := range ids {
		a, err := s.Load(id)
		if err != nil {
			if errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "no such file") {
				continue
			}
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
