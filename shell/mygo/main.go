// Command sasmita-desktop is the MyGo shell. Every call into the application
// goes through desktop.Bindings; swapping to the Wails-v3 fallback means
// replacing this module only.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/desktop"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model/adapters/openai"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
	"github.com/amaldevice/sasmita/internal/secrets"
	"github.com/egoist/mygo"
)

type shellService struct {
	bindings *desktop.Service
}

func (s *shellService) CreateConversation(ctx context.Context, projectID, title, harnessID string) (desktop.ConversationDTO, error) {
	return s.bindings.CreateConversation(ctx, projectID, title, harnessID)
}

func (s *shellService) ListConversations(ctx context.Context, projectID, cursor string, limit int) (desktop.ConversationPageDTO, error) {
	return s.bindings.ListConversations(ctx, projectID, cursor, limit)
}

func (s *shellService) ListRecentConversations(ctx context.Context, cursor string, limit int) (desktop.ConversationPageDTO, error) {
	return s.bindings.ListRecentConversations(ctx, cursor, limit)
}

func (s *shellService) GetConversation(ctx context.Context, conversationID string) (desktop.ConversationDetailDTO, error) {
	return s.bindings.GetConversation(ctx, conversationID)
}

func (s *shellService) GetRunSnapshot(ctx context.Context, runID string) (desktop.RunSnapshotDTO, error) {
	return s.bindings.GetRunSnapshot(ctx, runID)
}

// UI blocks fetch large datasets through this bounded, read-only API instead
// of receiving them in the prompt.
func (s *shellService) GetUIData(ctx context.Context, ref string, limit int) (desktop.UIDataDTO, error) {
	return s.bindings.GetUIData(ctx, ref, limit)
}

func (s *shellService) SaveUIState(ctx context.Context, runID, blockID string, patch json.RawMessage) error {
	return s.bindings.SaveUIState(ctx, runID, blockID, patch)
}

func (s *shellService) CancelRun(ctx context.Context, runID string) error {
	return s.bindings.CancelRun(ctx, runID)
}

func (s *shellService) ListRuns(ctx context.Context, conversationID string) ([]desktop.RunDTO, error) {
	return s.bindings.ListRuns(ctx, conversationID)
}

// Connection commands. Keys are accepted here only to be handed to the keychain
// through the application service; they never appear in a response DTO.
func (s *shellService) AddProviderProfile(ctx context.Context, req desktop.AddProviderProfileRequestDTO) (desktop.ProviderProfileDTO, error) {
	return s.bindings.AddProviderProfile(ctx, req)
}

func (s *shellService) ListProviderProfiles(ctx context.Context) ([]desktop.ProviderProfileDTO, error) {
	return s.bindings.ListProviderProfiles(ctx)
}

func (s *shellService) UpdateProviderProfile(ctx context.Context, req desktop.UpdateProviderProfileRequestDTO) (desktop.ProviderProfileDTO, error) {
	return s.bindings.UpdateProviderProfile(ctx, req)
}

func (s *shellService) DeleteProviderProfile(ctx context.Context, id string) error {
	return s.bindings.DeleteProviderProfile(ctx, id)
}

func (s *shellService) ListModels(ctx context.Context, profileID string) ([]desktop.ModelInfoDTO, error) {
	return s.bindings.ListModels(ctx, profileID)
}

func (s *shellService) TestConnection(ctx context.Context, profileID string) (desktop.ConnectionResultDTO, error) {
	return s.bindings.TestConnection(ctx, profileID)
}

// Onboarding commands. Detection uses a short protocol-aware probe; example
// prompts are static data from internal/onboarding.
func (s *shellService) ListExamplePrompts(ctx context.Context) []desktop.ExamplePromptDTO {
	return s.bindings.ListExamplePrompts(ctx)
}

func (s *shellService) DetectLocalEndpoints(ctx context.Context) ([]desktop.LocalEndpointDTO, error) {
	return s.bindings.DetectLocalEndpoints(ctx)
}

func (s *shellService) RenameConversation(ctx context.Context, conversationID, title string) (desktop.ConversationDTO, error) {
	return s.bindings.RenameConversation(ctx, conversationID, title)
}

func (s *shellService) MoveConversation(ctx context.Context, conversationID, projectID string) (desktop.ConversationDTO, error) {
	return s.bindings.MoveConversation(ctx, conversationID, projectID)
}

func (s *shellService) SearchConversations(ctx context.Context, query string, limit int) ([]desktop.SearchResultDTO, error) {
	return s.bindings.SearchConversations(ctx, query, limit)
}

func (s *shellService) ListProjects(ctx context.Context) ([]desktop.ProjectDTO, error) {
	return s.bindings.ListProjects(ctx)
}

// streamRun forwards RunEvents through a typed MyGo channel. The method stays
// pending until a terminal event, so the channel is never closed early.
func streamRun(ctx context.Context, handle desktop.RunHandleDTO, events *mygo.Channel[domain.RunEvent]) (string, error) {
	for {
		select {
		case ev, ok := <-handle.Events:
			if !ok {
				return handle.RunID, nil
			}
			if err := events.Send(ev); err != nil {
				handle.Abort()
				return handle.RunID, err
			}
			if ev.Type == domain.EventRunCompleted || ev.Type == domain.EventRunFailed || ev.Type == domain.EventRunCancelled {
				return handle.RunID, nil
			}
		case <-ctx.Done():
			// A disconnected UI must not kill an in-flight run; CancelRun is
			// the explicit cancellation command (TAD §14.3).
			return handle.RunID, ctx.Err()
		}
	}
}

// StartRun streams RunEvents through a typed MyGo channel.
func (s *shellService) StartRun(ctx context.Context, req desktop.StartRunRequestDTO, events *mygo.Channel[domain.RunEvent]) (string, error) {
	handle, err := s.bindings.StartRun(ctx, req)
	if err != nil {
		return "", err
	}
	return streamRun(ctx, handle, events)
}

// ResumeRun continues an interrupted run as a new attempt tied to the original.
func (s *shellService) ResumeRun(ctx context.Context, runID string, events *mygo.Channel[domain.RunEvent]) (string, error) {
	handle, err := s.bindings.ResumeRun(ctx, runID)
	if err != nil {
		return "", err
	}
	return streamRun(ctx, handle, events)
}

func setup(shell *shellService) error {
	dbPath := os.Getenv("SASMITA_DB_PATH")
	if dbPath == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		dbPath = filepath.Join(dir, "Sasmita", "sasmita.db")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return err
	}
	db, err := sqlite.Open(dbPath)
	if err != nil {
		return err
	}
	if err := migrations.Migrate(db); err != nil {
		return err
	}
	store := repositories.New(db)
	now := time.Now()
	// v1 has a single default Mode workspace; later tickets add project UI.
	_ = store.CreateProject(context.Background(), &domain.Project{
		ID: "p1", Name: "Default", CreatedAt: now, UpdatedAt: now,
	})

	baseURL := os.Getenv("SASMITA_OPENAI_BASE_URL")
	if baseURL == "" {
		return errors.New("SASMITA_OPENAI_BASE_URL is required")
	}
	modelRef := os.Getenv("SASMITA_MODEL")
	if modelRef == "" {
		modelRef = "gpt-4o-mini"
	}
	adapter := openai.Adapter{
		BaseURL:    baseURL,
		APIKey:     os.Getenv("SASMITA_OPENAI_API_KEY"),
		ProviderID: "openai",
	}
	artifactsDir := filepath.Join(os.TempDir(), "sasmita-artifacts")
	app := appsvc.New(store, appsvc.Config{
		Provider:     adapter,
		DefaultModel: modelRef,
		EnableTools:  true,
		ArtifactsDir: artifactsDir,
	})
	// Runs left active by a previous process cannot still be executing here.
	if _, err := app.RecoverStaleRuns(context.Background()); err != nil {
		return err
	}
	providerService := appsvc.NewProviderService(store, appsvc.ProviderServiceConfig{
		Secrets: secrets.NewKeychain(),
	})
	bindings := desktop.NewServiceWithProviders(app, providerService)
	bindings.SetUIDataDir(artifactsDir)
	shell.bindings = bindings
	return nil
}

func main() {
	shell := &shellService{}
	mygo.Bind(shell)
	mygo.App.WhenReady(func() {
		if err := setup(shell); err != nil {
			log.Printf("Sasmita setup failed: %v", err)
			return
		}
		mygo.NewWindow(mygo.WindowOptions{Title: "Sasmita", URL: "/"})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
