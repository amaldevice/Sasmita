// Package desktop is the only place that knows a desktop shell exists.
// MyGo (or the Wails-v3 fallback) implements Bindings; application code never
// imports a shell framework (TAD §18, ADR-001).
package desktop

import (
	"context"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/onboarding"
	"github.com/amaldevice/sasmita/internal/usage"
)

type ConversationDTO struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"projectId"`
	Title           string    `json:"title"`
	ActiveHarnessID string    `json:"activeHarnessId"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type ConversationPageDTO struct {
	Items      []ConversationDTO `json:"items"`
	NextCursor string            `json:"nextCursor"`
}

// ProjectDTO exposes only the fields the shell needs. An empty RootURI means
// the project has no folder (and neither do its conversations).
type ProjectDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	RootURI   string    `json:"rootUri"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SearchResultDTO is one full-text search hit; Snippet is a plain-text excerpt.
type SearchResultDTO struct {
	ConversationID string `json:"conversationId"`
	Title          string `json:"title"`
	ProjectID      string `json:"projectId"`
	Snippet        string `json:"snippet"`
}

// UsageDTO is the per-answer figure behind the hover popover. CostEstimated
// and TokenEstimated tell the UI never to present these as provider-reported.
type UsageDTO struct {
	ProviderID       string  `json:"providerId"`
	ModelID          string  `json:"modelId"`
	InputTokens      int64   `json:"inputTokens"`
	OutputTokens     int64   `json:"outputTokens"`
	CacheReadTokens  int64   `json:"cacheReadTokens"`
	CacheWriteTokens int64   `json:"cacheWriteTokens"`
	UncachedTokens   int64   `json:"uncachedTokens"`
	TokenEstimated   bool    `json:"tokenEstimated"`
	HitRate          float64 `json:"hitRate"`
	CostUSD          float64 `json:"costUsd"`
	CostEstimated    bool    `json:"costEstimated"`
	DurationMs       int64   `json:"durationMs"`
}

type MessageDTO struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Role           string    `json:"role"`
	Text           string    `json:"text"`
	CreatedAt      time.Time `json:"createdAt"`
	RunID          string    `json:"runId,omitempty"`
	Usage          *UsageDTO `json:"usage,omitempty"`
}

// UsageRowDTO is one per-model or per-day line in Settings › Diagnostics.
type UsageRowDTO struct {
	Key             string  `json:"key"`
	ProviderID      string  `json:"providerId,omitempty"`
	Requests        int64   `json:"requests"`
	InputTokens     int64   `json:"inputTokens"`
	OutputTokens    int64   `json:"outputTokens"`
	CacheReadTokens int64   `json:"cacheReadTokens"`
	HitRate         float64 `json:"hitRate"`
	CostUSD         float64 `json:"costUsd"`
	CostEstimated   bool    `json:"costEstimated"`
}

// UsageDropDTO is the only hit-rate warning the UI is allowed to show.
type UsageDropDTO struct {
	Day  string  `json:"day"`
	From float64 `json:"from"`
	To   float64 `json:"to"`
}

type UsageReportDTO struct {
	Models []UsageRowDTO `json:"models"`
	Days   []UsageRowDTO `json:"days"`
	Drop   *UsageDropDTO `json:"drop,omitempty"`
}

type ConversationDetailDTO struct {
	Conversation ConversationDTO `json:"conversation"`
	Messages     []MessageDTO    `json:"messages"`
}

type StartRunRequestDTO struct {
	ConversationID string `json:"conversationId"`
	Text           string `json:"text"`
	ModelRef       string `json:"modelRef"`
	HarnessID      string `json:"harnessId"`
}

// RunHandleDTO is a Go-side binding handle. A shell adapter drains Events into
// its own typed channel and exposes the JSON-safe fields to TypeScript.
type RunHandleDTO struct {
	RunID          string
	ConversationID string
	Events         <-chan domain.RunEvent
	Abort          func()
}

type RunSnapshotDTO struct {
	RunID  string            `json:"runId"`
	Status string            `json:"status"`
	Answer string            `json:"answer"`
	Events []domain.RunEvent `json:"events"`
}

// RunDTO is the UI-facing lifecycle record used to show interrupted runs and
// their continue hint (TAD §8.3).
type RunDTO struct {
	RunID          string `json:"runId"`
	ConversationID string `json:"conversationId"`
	Status         string `json:"status"`
	Answer         string `json:"answer"`
	OriginalRunID  string `json:"originalRunId,omitempty"`
}

// Bindings is the typed IPC surface exposed to the frontend.
type Bindings interface {
	CreateConversation(ctx context.Context, projectID, title, harnessID string) (ConversationDTO, error)
	ListConversations(ctx context.Context, projectID, cursor string, limit int) (ConversationPageDTO, error)
	ListRecentConversations(ctx context.Context, cursor string, limit int) (ConversationPageDTO, error)
	GetConversation(ctx context.Context, conversationID string) (ConversationDetailDTO, error)
	RenameConversation(ctx context.Context, conversationID, title string) (ConversationDTO, error)
	MoveConversation(ctx context.Context, conversationID, projectID string) (ConversationDTO, error)
	SearchConversations(ctx context.Context, query string, limit int) ([]SearchResultDTO, error)
	ListProjects(ctx context.Context) ([]ProjectDTO, error)
	StartRun(ctx context.Context, req StartRunRequestDTO) (RunHandleDTO, error)
	CancelRun(ctx context.Context, runID string) error
	ResumeRun(ctx context.Context, runID string) (RunHandleDTO, error)
	ListRuns(ctx context.Context, conversationID string) ([]RunDTO, error)
	GetRunSnapshot(ctx context.Context, runID string) (RunSnapshotDTO, error)
	AddProviderProfile(ctx context.Context, req AddProviderProfileRequestDTO) (ProviderProfileDTO, error)
	ListProviderProfiles(ctx context.Context) ([]ProviderProfileDTO, error)
	UpdateProviderProfile(ctx context.Context, req UpdateProviderProfileRequestDTO) (ProviderProfileDTO, error)
	DeleteProviderProfile(ctx context.Context, id string) error
	ListModels(ctx context.Context, profileID string) ([]ModelInfoDTO, error)
	TestConnection(ctx context.Context, profileID string) (ConnectionResultDTO, error)
	ListExamplePrompts(ctx context.Context) []ExamplePromptDTO
	DetectLocalEndpoints(ctx context.Context) ([]LocalEndpointDTO, error)
	GetUsageReport(ctx context.Context) (UsageReportDTO, error)
}

// Service adapts the application service to the typed bindings. It is shared
// by every shell implementation.
type Service struct {
	svc        *appsvc.Service
	providers  *appsvc.ProviderService
	localProbe onboarding.Probe
}

var _ Bindings = (*Service)(nil)

func NewService(svc *appsvc.Service) *Service {
	return &Service{svc: svc}
}

// NewServiceWithProviders wires the provider-profile commands in addition to
// the chat commands. NewService remains available for chat-only shells.
func NewServiceWithProviders(svc *appsvc.Service, providers *appsvc.ProviderService) *Service {
	return &Service{svc: svc, providers: providers}
}

func (s *Service) CreateConversation(ctx context.Context, projectID, title, harnessID string) (ConversationDTO, error) {
	conv, err := s.svc.CreateConversation(ctx, projectID, title, harnessID)
	if err != nil {
		return ConversationDTO{}, err
	}
	return conversationDTO(*conv), nil
}

func (s *Service) ListConversations(ctx context.Context, projectID, cursor string, limit int) (ConversationPageDTO, error) {
	convs, next, err := s.svc.ListConversations(ctx, projectID, cursor, limit)
	if err != nil {
		return ConversationPageDTO{}, err
	}
	items := make([]ConversationDTO, 0, len(convs))
	for _, conv := range convs {
		items = append(items, conversationDTO(conv))
	}
	return ConversationPageDTO{Items: items, NextCursor: next}, nil
}

func (s *Service) ListRecentConversations(ctx context.Context, cursor string, limit int) (ConversationPageDTO, error) {
	convs, next, err := s.svc.ListRecentConversations(ctx, cursor, limit)
	if err != nil {
		return ConversationPageDTO{}, err
	}
	items := make([]ConversationDTO, 0, len(convs))
	for _, conv := range convs {
		items = append(items, conversationDTO(conv))
	}
	return ConversationPageDTO{Items: items, NextCursor: next}, nil
}

func (s *Service) GetConversation(ctx context.Context, conversationID string) (ConversationDetailDTO, error) {
	detail, err := s.svc.GetConversation(ctx, conversationID)
	if err != nil {
		return ConversationDetailDTO{}, err
	}
	messages := make([]MessageDTO, 0, len(detail.Messages))
	for _, m := range detail.Messages {
		messages = append(messages, MessageDTO{
			ID: m.ID, ConversationID: m.ConversationID, Role: string(m.Role),
			Text: m.Text(), CreatedAt: m.CreatedAt, RunID: m.RunID,
			Usage: usageDTO(m.Usage),
		})
	}
	return ConversationDetailDTO{Conversation: conversationDTO(detail.Conversation), Messages: messages}, nil
}

func (s *Service) RenameConversation(ctx context.Context, conversationID, title string) (ConversationDTO, error) {
	conv, err := s.svc.RenameConversation(ctx, conversationID, title)
	if err != nil {
		return ConversationDTO{}, err
	}
	return conversationDTO(*conv), nil
}

func (s *Service) MoveConversation(ctx context.Context, conversationID, projectID string) (ConversationDTO, error) {
	conv, err := s.svc.MoveConversation(ctx, conversationID, projectID)
	if err != nil {
		return ConversationDTO{}, err
	}
	return conversationDTO(*conv), nil
}

func (s *Service) SearchConversations(ctx context.Context, query string, limit int) ([]SearchResultDTO, error) {
	hits, err := s.svc.SearchConversations(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]SearchResultDTO, 0, len(hits))
	for _, h := range hits {
		out = append(out, SearchResultDTO{
			ConversationID: h.ConversationID, Title: h.Title,
			ProjectID: h.ProjectID, Snippet: h.Snippet,
		})
	}
	return out, nil
}

func (s *Service) ListProjects(ctx context.Context) ([]ProjectDTO, error) {
	projects, err := s.svc.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectDTO, 0, len(projects))
	for _, p := range projects {
		out = append(out, ProjectDTO{
			ID: p.ID, Name: p.Name, RootURI: p.RootURI,
			CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Service) StartRun(ctx context.Context, req StartRunRequestDTO) (RunHandleDTO, error) {
	handle, err := s.svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: req.ConversationID,
		Text:           req.Text,
		ModelRef:       req.ModelRef,
		HarnessID:      req.HarnessID,
	})
	if err != nil {
		return RunHandleDTO{}, err
	}
	return RunHandleDTO{
		RunID:          handle.RunID,
		ConversationID: handle.ConversationID,
		Events:         handle.Events(),
		Abort:          handle.Abort,
	}, nil
}

func (s *Service) CancelRun(ctx context.Context, runID string) error {
	return s.svc.CancelRun(ctx, runID)
}

func (s *Service) ResumeRun(ctx context.Context, runID string) (RunHandleDTO, error) {
	handle, err := s.svc.ResumeRun(ctx, runID)
	if err != nil {
		return RunHandleDTO{}, err
	}
	return RunHandleDTO{
		RunID:          handle.RunID,
		ConversationID: handle.ConversationID,
		Events:         handle.Events(),
	}, nil
}

func (s *Service) ListRuns(ctx context.Context, conversationID string) ([]RunDTO, error) {
	runs, err := s.svc.ListRuns(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	out := make([]RunDTO, 0, len(runs))
	for _, run := range runs {
		out = append(out, RunDTO{
			RunID: run.ID, ConversationID: run.ConversationID, Status: string(run.Status),
			OriginalRunID: run.OriginalRunID,
		})
	}
	return out, nil
}

func (s *Service) GetRunSnapshot(ctx context.Context, runID string) (RunSnapshotDTO, error) {
	snapshot, err := s.svc.GetRunSnapshot(ctx, runID)
	if err != nil {
		return RunSnapshotDTO{}, err
	}
	return RunSnapshotDTO{
		RunID:  snapshot.Run.ID,
		Status: string(snapshot.Run.Status),
		Answer: snapshot.Answer,
		Events: snapshot.Events,
	}, nil
}

// GetUsageReport exposes the Diagnostics aggregation; the hit-rate warning is
// carried here and is not part of any chat read model.
func (s *Service) GetUsageReport(ctx context.Context) (UsageReportDTO, error) {
	report, err := s.svc.GetUsageReport(ctx)
	if err != nil {
		return UsageReportDTO{}, err
	}
	out := UsageReportDTO{
		Models: make([]UsageRowDTO, 0, len(report.Models)),
		Days:   make([]UsageRowDTO, 0, len(report.Days)),
	}
	for _, row := range report.Models {
		out.Models = append(out.Models, usageRowDTO(row))
	}
	for _, row := range report.Days {
		out.Days = append(out.Days, usageRowDTO(row))
	}
	if report.Drop != nil {
		out.Drop = &UsageDropDTO{Day: report.Drop.Day, From: report.Drop.From, To: report.Drop.To}
	}
	return out, nil
}

func usageRowDTO(row usage.Row) UsageRowDTO {
	return UsageRowDTO{
		Key: row.Key, ProviderID: row.ProviderID, Requests: row.Requests,
		InputTokens: row.InputTokens, OutputTokens: row.OutputTokens,
		CacheReadTokens: row.CacheReadTokens, HitRate: row.HitRate,
		CostUSD: row.CostUSD, CostEstimated: row.CostEstimated,
	}
}

func usageDTO(u *usage.AnswerUsage) *UsageDTO {
	if u == nil {
		return nil
	}
	return &UsageDTO{
		ProviderID: u.ProviderID, ModelID: u.ModelID,
		InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
		CacheReadTokens: u.CacheReadTokens, CacheWriteTokens: u.CacheWriteTokens,
		UncachedTokens: u.UncachedTokens, TokenEstimated: u.TokenEstimated,
		HitRate: u.HitRate, CostUSD: u.CostUSD,
		CostEstimated: u.CostEstimated, DurationMs: u.DurationMs,
	}
}

func conversationDTO(c domain.Conversation) ConversationDTO {
	return ConversationDTO{
		ID: c.ID, ProjectID: c.ProjectID, Title: c.Title,
		ActiveHarnessID: c.ActiveHarnessID, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}
