// Package appsvc implements the user-facing use cases from TAD §14.1.
// Wire logic stays in internal/model/adapters, never here (ADR-0002).
package appsvc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
)

const (
	defaultSystemPrompt = "You are Sasmita. Answer clearly and concisely."
	defaultModel        = "gpt-4o-mini"
	defaultHarness      = "chat"
)

type Config struct {
	Provider         model.ModelProvider
	DefaultModel     string
	CoalesceSize     int
	CoalesceInterval time.Duration // 0 disables timer-based flushing
	ContextLimit     int
	Now              func() time.Time
	NewID            func() string
}

type Service struct {
	store *repositories.Store
	cfg   Config
}

func New(store *repositories.Store, cfg Config) *Service {
	if cfg.CoalesceSize <= 0 {
		cfg.CoalesceSize = 32
	}
	if cfg.ContextLimit <= 0 {
		cfg.ContextLimit = 50
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
	return &Service{store: store, cfg: cfg}
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b[:])
}

func (s *Service) CreateConversation(ctx context.Context, projectID, title, harnessID string) (*domain.Conversation, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, domain.ErrInvalidArgument
	}
	if harnessID == "" {
		harnessID = defaultHarness
	}
	now := s.cfg.Now()
	conv := &domain.Conversation{
		ID:              s.cfg.NewID(),
		ProjectID:       projectID,
		Title:           title,
		ActiveHarnessID: harnessID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.store.CreateConversation(ctx, conv); err != nil {
		return nil, err
	}
	return conv, nil
}

func (s *Service) ListConversations(ctx context.Context, projectID, cursor string, limit int) ([]domain.Conversation, string, error) {
	return s.store.ListConversations(ctx, projectID, cursor, limit)
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
	return &domain.ConversationDetail{Conversation: *conv, Messages: messages}, nil
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
