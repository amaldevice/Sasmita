// Package harness owns the built-in Mode catalog. A Mode is a working
// environment: instructions, tools, skills, workspace, permissions and context
// policy. A model is one field selected for a run, not the Mode itself
// (GLOSSARY.md, TAD §10.1).
package harness

import "errors"

const (
	ModeChat       = "chat"
	ModeRiset      = "riset"
	ModePenulisan  = "penulisan"
	ModeAnalisData = "analis-data"
	ModeCoding     = "coding"
)

var (
	ErrUnknownMode     = errors.New("unknown Mode")
	ErrModeUnavailable = errors.New("Mode is not available in this phase")
)

// VisualFirstInstructions is product instruction text shared by every Mode
// (TAD §12A.15, ticket #30). A concept or process question gets a visual before
// its explanation; a short answer stays text. It is static, so it cannot
// perturb the byte-stable prefix: it is identical on every turn.
const VisualFirstInstructions = "For concept or process questions, call render_ui or create_artifact FIRST, then write the explanation after the tool result. Simple answers stay text."

// Mode is the declarative configuration of a working environment. Tool and
// skill IDs are references; their contents are never part of this catalog.
type Mode struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Instructions      string   `json:"instructions"`
	ToolsetIDs        []string `json:"toolsetIds"`
	SkillIDs          []string `json:"skillIds"`
	Workspace         string   `json:"workspace"`
	PermissionProfile string   `json:"permissionProfile"`
	ContextPolicy     string   `json:"contextPolicy"`
	Available         bool     `json:"available"`
	Phase             int      `json:"phase"`
}

// The Modes whose tools do not exist until later phases are listed but not
// selectable: Analis Data (Phase 4) and Coding (Phase 5).
var builtins = []Mode{
	{
		ID: ModeChat, Name: "Chat",
		Instructions: "", ToolsetIDs: []string{}, SkillIDs: []string{},
		Workspace: "none", PermissionProfile: "read-only", ContextPolicy: "full-history",
		Available: true, Phase: 1,
	},
	{
		ID: ModeRiset, Name: "Riset",
		Instructions: "Research the question carefully. Prefer verifiable sources and cite them with the answer.",
		ToolsetIDs:   []string{}, SkillIDs: []string{},
		Workspace: "none", PermissionProfile: "read-only", ContextPolicy: "full-history",
		Available: true, Phase: 2,
	},
	{
		ID: ModePenulisan, Name: "Penulisan",
		Instructions: "Draft and revise text with a clear outline. Keep sections distinct and avoid repetition.",
		ToolsetIDs:   []string{}, SkillIDs: []string{},
		Workspace: "none", PermissionProfile: "read-only", ContextPolicy: "full-history",
		Available: true, Phase: 3,
	},
	{
		ID: ModeAnalisData, Name: "Analis Data",
		Instructions: "Analyse tabular data with bounded queries and report the method used.",
		ToolsetIDs:   []string{"query_table"}, SkillIDs: []string{},
		Workspace: "attached", PermissionProfile: "data-worker", ContextPolicy: "full-history",
		Available: false, Phase: 4,
	},
	{
		ID: ModeCoding, Name: "Coding",
		Instructions: "Work in the project workspace. Run tests and explain changes before editing files.",
		ToolsetIDs:   []string{"file_read", "shell"}, SkillIDs: []string{},
		Workspace: "project", PermissionProfile: "sandboxed-shell", ContextPolicy: "full-history",
		Available: false, Phase: 5,
	},
}

// List returns a copy of the built-in catalog, including unavailable Modes so
// the UI can show them as "coming later".
func List() []Mode {
	return append([]Mode(nil), builtins...)
}

// Select resolves an ID (empty means Chat) and rejects Modes that are not
// available in v1.
func Select(id string) (Mode, error) {
	if id == "" {
		id = ModeChat
	}
	for _, mode := range builtins {
		if mode.ID == id {
			if !mode.Available {
				return Mode{}, ErrModeUnavailable
			}
			return mode, nil
		}
	}
	return Mode{}, ErrUnknownMode
}
