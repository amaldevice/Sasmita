// Package desktop is the only place that knows a desktop shell exists.
// MyGo (or the Wails-v3 fallback) implements Bindings; application code never
// imports a shell framework (TAD §18, ADR-001).
package desktop

import (
	"context"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/domain"
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

type MessageDTO struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Role           string    `json:"role"`
	Text           string    `json:"text"`
	CreatedAt      time.Time `json:"createdAt"`
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
	Cancel         func()
	Abort          func()
}

type RunSnapshotDTO struct {
	RunID  string            `json:"runId"`
	Status string            `json:"status"`
	Answer string            `json:"answer"`
	Events []domain.RunEvent `json:"events"`
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
	GetRunSnapshot(ctx context.Context, runID string) (RunSnapshotDTO, error)
}

// Service adapts the application service to the typed bindings. It is shared
// by every shell implementation.
type Service struct {
	svc *appsvc.Service
}

var _ Bindings = (*Service)(nil)

func NewService(svc *appsvc.Service) *Service {
	return &Service{svc: svc}
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
			Text: m.Text(), CreatedAt: m.CreatedAt,
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
		Cancel:         handle.Cancel,
		Abort:          handle.Abort,
	}, nil
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

func conversationDTO(c domain.Conversation) ConversationDTO {
	return ConversationDTO{
		ID: c.ID, ProjectID: c.ProjectID, Title: c.Title,
		ActiveHarnessID: c.ActiveHarnessID, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}
