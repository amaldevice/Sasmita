// Package appsvc implements the user-facing use cases from TAD §14.1.
// Wire logic stays in internal/model/adapters, never here (ADR-0002).
package appsvc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/amaldevice/sasmita/internal/agent"
	"github.com/amaldevice/sasmita/internal/artifacts"
	ctxlayout "github.com/amaldevice/sasmita/internal/context"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/genui"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/policy"
	"github.com/amaldevice/sasmita/internal/runtime"
	"github.com/amaldevice/sasmita/internal/tools"
	"github.com/amaldevice/sasmita/internal/tools/discovery"
	"github.com/amaldevice/sasmita/internal/usage"
)

const (
	defaultModel = "gpt-4o-mini"
	defaultMode  = "chat"
)

// ErrProviderUnavailable means a run named a provider profile but the service
// has no resolver (a chat-only shell). It fails before any message is stored.
var ErrProviderUnavailable = errors.New("provider profile resolution is not configured")

type Config struct {
	Provider         model.ModelProvider
	DefaultModel     string
	CoalesceSize     int
	CoalesceInterval time.Duration // 0 disables timer-based flushing
	Tools            []model.ToolDefinition
	// EnableTools opts a service into the built-in toolset (#22). Tool schemas
	// are frozen for the service lifetime; without it the wire is unchanged.
	EnableTools    bool
	ArtifactsDir   string
	WorkspaceRoots []string
	AttachedFiles  []string
	ToolOutputCap  int
	Agent          agent.Engine
	// Policy and Approvals wire the TAD §15.3 engine into the agent tool loop.
	// A nil Policy gets the default matrix; nil Approvals gets a SQLite-backed
	// coordinator sharing the store passed to New.
	Policy    *policy.Engine
	Approvals *runtime.ApprovalCoordinator
	// Artifacts opts the service into Tier 2 artifact commands and tools (#28).
	Artifacts *artifacts.Service
	// ContextNote supplies dynamic information for the tail note on the new
	// user turn. It must never be placed in the static system instruction.
	ContextNote func() string
	// NoteSource is the delta-aware replacement for ContextNote. The builder
	// emits only what changed since the previous note (GLOSSARY "context note").
	NoteSource func() ctxlayout.NoteInput
	// InputBudgetTokens enables automatic compaction at Threshold (0.75 by
	// default) of the input budget; zero disables it.
	InputBudgetTokens   int
	CompactionThreshold float64
	KeepUserTurns       int
	Now                 func() time.Time
	NewID               func() string
	Runtime             *runtime.Manager
}

type Service struct {
	store        *repositories.Store
	cfg          Config
	builder      ctxlayout.Builder
	notes        *ctxlayout.NoteBuilder
	compactor    ctxlayout.Compactor
	runtime      *runtime.Manager
	approvals    *runtime.ApprovalCoordinator
	providerKind domain.ProviderKind
	providerFor  func(context.Context, string) (model.ModelProvider, error)
	genui        *genui.Store
	discovery    *discovery.Service
}

// SetProviderResolver wires profile-based adapter resolution. The shell that
// owns provider profiles calls this once during setup; chat-only shells keep
// their single configured adapter.
func (s *Service) SetProviderResolver(resolve func(context.Context, string) (model.ModelProvider, error)) {
	s.providerFor = resolve
}

func New(store *repositories.Store, cfg Config) *Service {
	if cfg.CoalesceSize <= 0 {
		cfg.CoalesceSize = 32
	}
	if cfg.DefaultModel == "" {
		cfg.DefaultModel = defaultModel
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.NewID == nil {
		cfg.NewID = randomID
	}
	if cfg.Approvals == nil {
		cfg.Approvals = runtime.NewApprovalCoordinator(store, cfg.Policy)
	}
	var gen *genui.Store
	var disc *discovery.Service
	if cfg.Agent == nil {
		var registry *tools.Registry
		if cfg.EnableTools {
			registry = tools.New(tools.Config{
				ArtifactsDir: cfg.ArtifactsDir, WorkspaceRoots: cfg.WorkspaceRoots,
				AttachedFiles: cfg.AttachedFiles, OutputCap: cfg.ToolOutputCap,
				Search: store, NewID: cfg.NewID, Artifacts: cfg.Artifacts,
			})
			// Tier 1 GenUI reuses #22's registry and tool engine. The host store
			// keeps validated specs out of the prompt and out of tool results.
			gen = genui.NewStore()
			registry.Register(genui.NewRenderTool(gen))
			registry.Register(genui.NewSkillTool())
			// Meta-tools join the frozen list before it is captured. No
			// discovery-only tools are enabled in v1 (query_table is Phase 4).
			disc = discovery.New(registry, discovery.DispatcherConfig{})
			cfg.Tools = registry.Definitions()
		}
		cfg.Agent = agent.New(agent.Config{
			Provider: cfg.Provider, Tools: registry, GenUI: gen, NewID: cfg.NewID,
			States: store, Approvals: approvalGateAdapter{coordinator: cfg.Approvals},
			Discovery: disc,
		})
	}
	// Freeze tools for the lifetime of the service (one conversation per run).
	cfg.Tools = append([]model.ToolDefinition(nil), cfg.Tools...)
	if cfg.Runtime == nil {
		cfg.Runtime = runtime.NewManager()
	}
	// Undeclared providers keep every raw block (the pre-switch behaviour).
	var kind domain.ProviderKind
	if aware, ok := cfg.Provider.(model.ProviderKindAware); ok {
		kind = domain.ProviderKind(aware.ProviderKind())
	}
	builder := ctxlayout.Builder{Tools: cfg.Tools}
	return &Service{
		store:   store,
		cfg:     cfg,
		builder: builder,
		notes:   ctxlayout.NewNoteBuilder(),
		compactor: ctxlayout.Compactor{
			Provider:          cfg.Provider,
			Builder:           builder,
			InputBudgetTokens: cfg.InputBudgetTokens,
			Threshold:         cfg.CompactionThreshold,
			KeepUserTurns:     cfg.KeepUserTurns,
			NewID:             cfg.NewID,
			Now:               cfg.Now,
		},
		runtime:      cfg.Runtime,
		approvals:    cfg.Approvals,
		providerKind: kind,
		genui:        gen,
		discovery:    disc,
	}
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b[:])
}

func (s *Service) CreateConversation(ctx context.Context, projectID, title, modeID string) (*domain.Conversation, error) {
	// An empty project id is valid: the conversation is not in a project and
	// therefore has no folder to open (migrations/003_init.sql).
	if modeID == "" {
		modeID = defaultMode
	}
	now := s.cfg.Now()
	conv := &domain.Conversation{
		ID:           s.cfg.NewID(),
		ProjectID:    projectID,
		Title:        title,
		ActiveModeID: modeID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.store.CreateConversation(ctx, conv); err != nil {
		return nil, err
	}
	return conv, nil
}

func (s *Service) ListConversations(ctx context.Context, projectID, cursor string, limit int) ([]domain.Conversation, string, error) {
	return s.store.ListConversations(ctx, projectID, cursor, limit)
}

func (s *Service) ListRecentConversations(ctx context.Context, cursor string, limit int) ([]domain.Conversation, string, error) {
	return s.store.ListRecentConversations(ctx, cursor, limit)
}

// GetConversation returns the conversation together with its canonical history
// so the UI can rebuild state after a restart.
func (s *Service) GetConversation(ctx context.Context, id string) (*domain.ConversationDetail, error) {
	conv, err := s.store.GetConversation(ctx, id)
	if err != nil {
		return nil, err
	}
	messages, err := s.store.ListMessages(ctx, id, 0)
	if err != nil {
		return nil, err
	}
	// Attach each answer's usage from the run that produced it. This read path
	// never returns a hit-rate warning: that belongs to Diagnostics only.
	for i := range messages {
		m := &messages[i]
		if m.Role != domain.RoleAssistant || m.RunID == "" {
			continue
		}
		usages, err := s.store.ListUsage(ctx, m.RunID)
		if err != nil || len(usages) == 0 {
			continue
		}
		u := usages[len(usages)-1]
		view := usage.ViewOf(usage.Record{
			ProviderID: u.ProviderID, ModelID: u.ModelID,
			InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
			CacheReadTokens: u.CacheReadTokens, CacheWriteTokens: u.CacheWriteTokens,
			UncachedTokens: u.UncachedTokens, Estimated: u.Estimated,
			CostEstimate: u.CostEstimate,
		})
		if run, err := s.store.GetRun(ctx, m.RunID); err == nil && run.FinishedAt != nil {
			view.DurationMs = run.FinishedAt.Sub(run.StartedAt).Milliseconds()
		}
		m.Usage = &view
	}
	return &domain.ConversationDetail{Conversation: *conv, Messages: messages}, nil
}

// ListRuns exposes run lifecycle state so the UI can offer Continue after an
// interrupted run is recovered at startup.
func (s *Service) ListRuns(ctx context.Context, conversationID string) ([]domain.Run, error) {
	return s.store.ListRuns(ctx, conversationID)
}

// RenameConversation changes only the title; canonical history is untouched.
func (s *Service) RenameConversation(ctx context.Context, id, title string) (*domain.Conversation, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, domain.ErrInvalidArgument
	}
	if err := s.store.RenameConversation(ctx, id, title, s.cfg.Now()); err != nil {
		return nil, err
	}
	return s.store.GetConversation(ctx, id)
}

// MoveConversation associates a conversation with a project. domain.NoProjectID
// removes the association; such conversations have no folder access.
func (s *Service) MoveConversation(ctx context.Context, id, projectID string) (*domain.Conversation, error) {
	if err := s.store.MoveConversation(ctx, id, projectID, s.cfg.Now()); err != nil {
		return nil, err
	}
	return s.store.GetConversation(ctx, id)
}

func (s *Service) SearchConversations(ctx context.Context, query string, limit int) ([]domain.SearchResult, error) {
	return s.store.SearchConversations(ctx, query, limit)
}

func (s *Service) ListProjects(ctx context.Context) ([]domain.Project, error) {
	return s.store.ListProjects(ctx)
}

// GetRunSnapshot supports reconnect recovery (TAD §14.3).
func (s *Service) GetRunSnapshot(ctx context.Context, runID string) (*domain.RunSnapshot, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	events, err := s.store.ListRunEvents(ctx, runID)
	if err != nil {
		return nil, err
	}
	var answer strings.Builder
	for _, ev := range events {
		if ev.Type != domain.EventModelTextDelta {
			continue
		}
		var payload domain.TextDeltaPayload
		if json.Unmarshal(ev.Payload, &payload) == nil {
			answer.WriteString(payload.Text)
		}
	}
	return &domain.RunSnapshot{Run: *run, Events: events, Answer: answer.String()}, nil
}
