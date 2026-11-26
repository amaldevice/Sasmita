package domain

import "encoding/json"

// Tool events extend the versioned RunEvent stream. Approval decisions belong
// to the later policy engine (#23); these events only describe what the agent
// proposed and what it actually executed so that engine has something to act on.
const (
	EventToolProposed = "tool.proposed"
	EventToolExecuted = "tool.executed"
	EventToolArtifact = "tool.artifact"
	EventToolFailed   = "tool.failed"
)

// AgentSignalKind identifies a normalized agent-runtime signal. No provider or
// ADK type is visible at this boundary.
type AgentSignalKind string

const (
	AgentTextDelta    AgentSignalKind = "text_delta"
	AgentUsage        AgentSignalKind = "usage"
	AgentToolProposed AgentSignalKind = "tool_proposed"
	AgentToolExecuted AgentSignalKind = "tool_executed"
	AgentToolArtifact AgentSignalKind = "tool_artifact"
	AgentToolFailed   AgentSignalKind = "tool_failed"
	AgentToolPair     AgentSignalKind = "tool_pair"
)

// AgentSignal is one observable step of an agent run. Text and usage signals
// feed the coalesced answer; tool signals carry both UI detail and the raw
// blocks that must be appended to canonical history.
type AgentSignal struct {
	Kind  AgentSignalKind
	Text  string
	Usage *Usage
	Tool  ToolActivity
}

// ToolActivity is the provider-neutral description of one tool step.
// Executed distinguishes a proposal (policy has not allowed it yet) from an
// executed call; the policy engine consumes that distinction in #23.
type ToolActivity struct {
	CallID          string
	Name            string
	Risk            string
	Input           json.RawMessage
	Output          string
	Summary         string
	ArtifactID      string
	ArtifactRef     string
	Truncated       bool
	Executed        bool
	DurationMS      int64
	Error           string
	AssistantBlocks []json.RawMessage
	ResultBlocks    []json.RawMessage
}

// ToolPayload is carried by every tool.* RunEvent (TAD §14.2 envelope).
type ToolPayload struct {
	Envelope
	CallID      string          `json:"callId,omitempty"`
	Name        string          `json:"name"`
	Risk        string          `json:"risk,omitempty"`
	Input       json.RawMessage `json:"input,omitempty"`
	Output      string          `json:"output,omitempty"`
	Summary     string          `json:"summary,omitempty"`
	ArtifactRef string          `json:"artifactRef,omitempty"`
	Truncated   bool            `json:"truncated,omitempty"`
	Executed    bool            `json:"executed"`
	DurationMS  int64           `json:"durationMs,omitempty"`
	Status      string          `json:"status,omitempty"`
	Error       string          `json:"error,omitempty"`
}
