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
	"github.com/amaldevice/sasmita/internal/artifacts"
	"github.com/amaldevice/sasmita/internal/desktop"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters/openai"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
	"github.com/amaldevice/sasmita/internal/secrets"
	"github.com/egoist/mygo"
)

type shellService struct {
	bindings *desktop.Service
	app      *appsvc.Service
	live     liveReporter
	notifier desktopNotifier
}

func (s *shellService) CreateConversation(ctx context.Context, projectID, title, modeID string) (desktop.ConversationDTO, error) {
	return s.bindings.CreateConversation(ctx, projectID, title, modeID)
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

// NotifyApproval is the one-way binding used by the renderer's focus-gated
// hook. The Go side checks frontmost state again, so a stale renderer cannot
// produce a notification while Sasmita is in the foreground.
func (s *shellService) NotifyApproval(_ context.Context, title, body string) error {
	return notifyApproval(s.notifier, title, body)
}

// CompactConversation is the user-requested compaction command; canonical
// history is never rewritten (GLOSSARY "compaction").
func (s *shellService) CompactConversation(ctx context.Context, conversationID string) (desktop.CompactionDTO, error) {
	return s.bindings.CompactConversation(ctx, conversationID)
}

// PickAttachments opens the native file picker and returns absolute paths.
// The browser never sees file handles; tools receive only these explicit roots.
func (s *shellService) PickAttachments(ctx context.Context) ([]string, error) {
	return mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: mygo.CallerWindow(ctx), Title: "Attach files", Multiple: true,
	})
}

// Diagnostics usage report. Hit-rate warnings live only in this command.
func (s *shellService) GetUsageReport(ctx context.Context) (desktop.UsageReportDTO, error) {
	return s.bindings.GetUsageReport(ctx)
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

// The shipped model's input context. A zero budget would leave ShouldCompact
// inert no matter how well the compactor is tested in isolation (#24 review).
const inputBudgetTokens = 128_000

func appConfig(provider model.ModelProvider, modelRef, artifactsDir string, artifactService *artifacts.Service) appsvc.Config {
	return appsvc.Config{
		Provider:            provider,
		DefaultModel:        modelRef,
		EnableTools:         true,
		ArtifactsDir:        artifactsDir,
		Artifacts:           artifactService,
		InputBudgetTokens:   inputBudgetTokens,
		CompactionThreshold: 0.75,
	}
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
	// Tier 2 artifacts: payloads live next to the DB; metadata is mirrored into
	// it. The read-only scheme handler must be registered before any window.
	artifactStore := artifacts.NewStore(filepath.Join(filepath.Dir(dbPath), "artifacts"))
	artifactStore.DB = db
	artifactService := &artifacts.Service{Store: artifactStore}
	if err := mygo.Protocol.Handle(artifacts.Scheme, artifacts.NewHandler(artifactStore)); err != nil {
		return err
	}
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
	app := appsvc.New(store, appConfig(adapter, modelRef, artifactsDir, artifactService))
	// Runs left active by a previous process cannot still be executing here.
	if _, err := app.RecoverStaleRuns(context.Background()); err != nil {
		return err
	}
	providerService := appsvc.NewProviderService(store, appsvc.ProviderServiceConfig{
		Secrets: secrets.NewKeychain(),
	})
	// A run carrying a profile ID resolves its base URL and keychain secret
	// from this service instead of using the shell's default adapter.
	app.SetProviderResolver(providerService.AdapterFor)
	bindings := desktop.NewServiceWithProviders(app, providerService)
	bindings.SetUIDataDir(artifactsDir)
	shell.bindings = bindings
	shell.app = app
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
		win := mygo.NewWindow(mygo.WindowOptions{Title: "Sasmita", URL: "/"})
		shell.notifier = mygoNotifier{window: win}
		// MyGo v0.10 treats custom schemes as trusted in the main frame.
		// NavigateEvent has no frame flag; iframe loads of artifact:// are not
		// user-initiated, so blocking user-initiated ones keeps inline
		// artifacts working while stopping link clicks out of the trusted UI.
		win.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
			if e.UserInitiated && artifacts.RejectMainFrameNavigation(e.URL) != nil {
				log.Printf("blocked navigation to %s", e.URL)
				e.PreventDefault()
			}
		})
		startWatchdog(context.Background(), win.Page(), shell.app, &shell.live)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
