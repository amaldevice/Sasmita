// Package policy evaluates a proposed tool call before anything executes.
// The model-facing tool schema is not authorization: the real tool ID, the
// exact arguments, the project scope and the run's Mode are evaluated here
// (TAD §15.2/§15.3, ADR-0003). It has no SQL or desktop dependency.
package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/amaldevice/sasmita/internal/model"
)

// Mode is a working environment from the Mode catalog (GLOSSARY.md). Policy
// keeps its own small copy so it does not depend on the catalog's wiring.
type Mode string

const (
	ModeChat       Mode = "chat"
	ModeRiset      Mode = "riset"
	ModePenulisan  Mode = "penulisan"
	ModeAnalisData Mode = "analis-data"
	ModeCoding     Mode = "coding"
)

// Scope is the resource boundary of one run. Roots are canonical project
// directories plus explicitly attached files.
type Scope struct {
	ProjectID string
	Roots     []string
}

// Request is the complete input to a policy decision. Arguments are the exact
// bytes proposed by the model; they are hashed, never trusted as text.
type Request struct {
	ToolID    string
	Arguments json.RawMessage
	Mode      Mode
	Scope     Scope
}

type Outcome string

const (
	Allow    Outcome = "allow"
	Deny     Outcome = "deny"
	Approval Outcome = "approval"
)

// Decision carries both the outcome and the plain words shown on the approval
// card: what will happen, to which resource, and what it can change.
type Decision struct {
	Outcome  Outcome
	Reason   string
	Action   string
	Resource string
	Impact   string
}

// Snapshot is bound to every approval. A policy change invalidates old grants.
type Snapshot struct {
	Version string `json:"version"`
	Hash    string `json:"hash"`
}

// Engine is the v1 default matrix from TAD §15.3. Version participates in the
// snapshot hash so a policy change cannot be replayed under an old decision.
type Engine struct {
	Version string
}

func New() *Engine { return &Engine{Version: "v1"} }

// Evaluate applies capability, scope, operation, risk and isolation rules.
// hasGrant is a project-local "always allow" decision from a previous approval;
// it never overrides a hard deny and never covers shell execution.
func (e *Engine) Evaluate(req Request, hasGrant bool) Decision {
	version := e.Version
	if version == "" {
		version = "v1"
	}
	var args struct {
		Path      string `json:"path"`
		Name      string `json:"name"`
		Overwrite bool   `json:"overwrite"`
		Command   string `json:"command"`
	}
	_ = json.Unmarshal(req.Arguments, &args)
	d := func(outcome Outcome, reason, action, resource, impact string) Decision {
		return Decision{Outcome: outcome, Reason: reason, Action: action, Resource: resource, Impact: impact}
	}

	switch req.ToolID {
	case "calculator":
		return d(Allow, "bounded arithmetic", "Menghitung", "ekspresi aritmetika", "Hanya menghasilkan angka; tidak menyentuh berkas atau jaringan.")
	case "search":
		return d(Allow, "bounded read", "Mencari", "riwayat percakapan dan berkas proyek", "Hanya membaca; hasil dibatasi jumlah dan ukurannya.")
	case "read_project_file":
		path := strings.TrimSpace(args.Path)
		if path == "" {
			return d(Deny, "path is required", "Membaca berkas", "(tidak ada berkas)", "Tidak ada berkas yang akan dibaca.")
		}
		if !withinScope(path, req.Scope.Roots) {
			return d(Deny, "path is outside the approved workspace", "Membaca berkas", path, "Ditolak karena berada di luar workspace yang disetujui.")
		}
		return d(Allow, "read within approved workspace", "Membaca berkas", path, "Hanya membaca isi berkas ke dalam jawaban; berkas tidak diubah.")
	case "write_artifact":
		resource := strings.TrimSpace(args.Name)
		if resource == "" {
			resource = "artifact baru"
		}
		if args.Overwrite {
			d := d(Approval, "overwrite of an existing user file", "Menimpa artifact", resource, "Isi lama akan diganti; berikan persetujuan hanya jika itu yang diinginkan.")
			if hasGrant {
				d.Outcome, d.Reason = Allow, "project grant covers this overwrite"
			}
			return d
		}
		return d(Allow, "new reversible artifact", "Menyimpan artifact", resource, "Membuat artifact baru di workspace; berkas yang ada tidak ditimpa.")
	case "shell", "run_script":
		if req.Mode != ModeCoding {
			return d(Deny, "shell execution is only available in Coding Mode", "Menjalankan perintah", short(args.Command), "Tidak dijalankan: Mode ini tidak mengizinkan shell.")
		}
		return d(Approval, "shell execution requires per-command approval", "Menjalankan perintah", short(args.Command), "Perintah dapat mengubah berkas di workspace; izin ini tidak bisa dibuat selamanya.")
	case "install_dependencies", "publish", "send_email", "purchase", "external_write":
		d := d(Approval, "external side effect", "Mengirim ke luar aplikasi", req.ToolID, "Data akan keluar dari aplikasi; tidak bisa dibatalkan setelah dikirim.")
		if hasGrant {
			d.Outcome, d.Reason = Allow, "project grant covers this operation"
		}
		return d
	case "delete_many", "delete_user_file", "move_many":
		return d(Deny, "bulk or destructive change is denied by default", "Menghapus atau memindahkan", req.ToolID, "Ditolak karena dapat merusak pekerjaan pengguna.")
	case "read_secret_store", "read_secret":
		return d(Deny, "secrets stay outside context", "Membaca kredensial", req.ToolID, "Ditolak: kredensial tidak pernah masuk ke jawaban atau tool.")
	default:
		return d(Deny, "unknown tool fails closed", "Menjalankan tool", req.ToolID, "Tidak ada aturan yang mengizinkan tool ini.")
	}
}

// Snapshot describes the policy currently in force. The hash is deterministic
// so two fresh engines agree and a version change is observable.
func (e *Engine) Snapshot() Snapshot {
	version := e.Version
	if version == "" {
		version = "v1"
	}
	raw, _ := model.MarshalDeterministic(map[string]string{
		"version": version, "matrix": "tad-15.3-v1",
	})
	sum := sha256.Sum256(raw)
	return Snapshot{Version: version, Hash: hex.EncodeToString(sum[:])}
}

// HashArguments canonicalizes JSON so key order does not change an approval
// binding, while any change to a value does.
func HashArguments(args []byte) string {
	var value any
	if err := json.Unmarshal(args, &value); err != nil {
		value = string(args)
	}
	canonical, err := model.MarshalDeterministic(value)
	if err != nil {
		canonical = args
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// withinScope allows relative paths under a root and explicit absolute paths
// that are themselves allowed. Lexical containment is enough here: the tool
// executor resolves symlinks again before any file is opened.
func withinScope(path string, roots []string) bool {
	if len(roots) == 0 {
		return false
	}
	candidates := []string{path}
	if !filepath.IsAbs(path) {
		for _, root := range roots {
			candidates = append(candidates, filepath.Join(root, filepath.FromSlash(path)))
		}
	}
	for _, base := range roots {
		absBase, err := filepath.Abs(base)
		if err != nil {
			continue
		}
		for _, candidate := range candidates {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				continue
			}
			rel, err := filepath.Rel(absBase, abs)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			return true
		}
	}
	return false
}

func short(s string) string {
	const max = 80
	s = strings.TrimSpace(s)
	if len(s) > max {
		return fmt.Sprintf("%s…", s[:max])
	}
	if s == "" {
		return "(perintah tidak disebutkan)"
	}
	return s
}
