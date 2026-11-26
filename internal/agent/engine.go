// Package agent is the application-owned agent runtime. Eino ADK supplies the
// run lifecycle (Runner, Agent, WithCancel, checkpoint Get/Set) behind this
// package; no ADK or provider SDK type is visible at the Engine boundary.
// The model wire format stays in app-owned adapters (ADR-0002), so the ADK loop
// drives our own provider port instead of re-serializing messages itself.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	ctxlayout "github.com/amaldevice/sasmita/internal/context"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/genui"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/tools"
	"github.com/amaldevice/sasmita/internal/tools/discovery"
)

// Engine is the only surface appsvc sees. Its inputs and outputs are domain
// and app-owned model types, never Eino types.
type Engine interface {
	Run(ctx context.Context, req Request, emit func(domain.AgentSignal)) (Result, error)
}

type Request struct {
	ConversationID string
	RunID          string
	CheckpointID   string // stable across a retry, so an approval survives restart
	Mode           string
	ProjectID      string
	ScopeRoots     []string
	ModelRef       string
	History        []domain.Message
	// Provider selects the adapter for this run. Nil keeps the engine's
	// configured adapter (profile-based shells resolve one adapter per run).
	Provider model.ModelProvider
}

type Result struct {
	Text   string
	Blocks []json.RawMessage
	Usage  *model.Usage
}

// CheckpointStore is the app-owned shape of ADK's Get/Set([]byte) store. The
// interfaces are structurally identical, so no adapter is needed.
type CheckpointStore interface {
	Get(ctx context.Context, checkPointID string) ([]byte, bool, error)
	Set(ctx context.Context, checkPointID string, checkPoint []byte) error
}

type Config struct {
	Provider    model.ModelProvider
	Tools       *tools.Registry
	GenUI       *genui.Store
	MaxTurns    int
	Checkpoints CheckpointStore // Eino ADK checkpoint bytes
	States      CheckpointStore // app-owned pause state, under its own key prefix
	Approvals   ApprovalGate
	Discovery   *discovery.Service
	NewID       func() string
}

func New(cfg Config) Engine {
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 4
	}
	definitions := []model.ToolDefinition(nil)
	if cfg.Tools != nil {
		definitions = cfg.Tools.Definitions()
	}
	return &engine{
		cfg:     cfg,
		builder: ctxlayout.Builder{Tools: definitions},
	}
}

type engine struct {
	cfg     Config
	builder ctxlayout.Builder
}

func (e *engine) Run(ctx context.Context, req Request, emit func(domain.AgentSignal)) (Result, error) {
	definitions := e.builder.Tools
	if e.cfg.Discovery != nil {
		definitions = e.cfg.Discovery.Definitions(req.ConversationID)
	}
	loop := &loopAgent{
		engine: e, req: req, emit: emit,
		working: append([]domain.Message(nil), req.History...),
		builder: ctxlayout.Builder{Tools: definitions},
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent: loop, CheckPointStore: e.cfg.Checkpoints,
	})
	cancelOpt, cancelFn := adk.WithCancel()
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			cancelFn()
		case <-stop:
		}
	}()
	defer close(stop)

	opts := []adk.AgentRunOption{cancelOpt}
	if req.RunID != "" {
		opts = append(opts, adk.WithCheckPointID(req.RunID))
	}
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("agent run")}, opts...)
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			// ADK reports its own cancellation event; map it back to the
			// context error the run state machine (#21) already understands.
			if ctx.Err() != nil {
				return Result{}, ctx.Err()
			}
			var cancelled *adk.CancelError
			if errors.As(ev.Err, &cancelled) {
				return Result{}, context.Canceled
			}
			return Result{}, ev.Err
		}
		if ev.Output == nil {
			continue
		}
		switch v := ev.Output.CustomizedOutput.(type) {
		case domain.AgentSignal:
			emit(v)
		case Result:
			return v, nil
		}
	}
	return Result{}, fmt.Errorf("agent run ended without a result")
}
