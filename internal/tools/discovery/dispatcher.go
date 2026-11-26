package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/tools"
)

// Egress labels describe where a tool's effects stay. The same dispatcher is
// reusable by a future run_script; its label is supplied by the caller.
const (
	EgressNone    = "none"
	EgressLocal   = "local"
	EgressNetwork = "network"
)

// Executor is the narrow capability the dispatcher needs; it is satisfied by
// *tools.Registry and keeps discovery independent of registry internals.
type Executor interface {
	Spec(name string) (tools.Spec, bool)
	Execute(ctx context.Context, call model.ToolCall) (tools.Result, error)
}

// DispatcherConfig sets bounded concurrency, a hard per-call timeout and
// egress labels. Zero values mean one call per tool and no extra timeout.
type DispatcherConfig struct {
	DefaultConcurrency int
	Concurrency        map[string]int
	CallTimeout        time.Duration
	Egress             map[string]string
	OnCall             func(tool, egress string)
}

// Dispatcher validates every call in Go, bounds concurrency per tool and
// attaches an egress label. It never holds a tool's result bytes.
type Dispatcher struct {
	exec    Executor
	cfg     DispatcherConfig
	mu   sync.Mutex
	sems map[string]chan struct{}
}

func NewDispatcher(exec Executor, cfg DispatcherConfig) *Dispatcher {
	if cfg.DefaultConcurrency <= 0 {
		cfg.DefaultConcurrency = 1
	}
	return &Dispatcher{exec: exec, cfg: cfg, sems: make(map[string]chan struct{})}
}

// Call runs one validated tool call. Schema validation happens before any
// semaphore is taken, so an invalid call cannot consume a concurrency slot.
func (d *Dispatcher) Call(ctx context.Context, name string, args json.RawMessage) (tools.Result, error) {
	spec, ok := d.exec.Spec(name)
	if !ok {
		return tools.Result{}, fmt.Errorf("tool %q is not enabled", name)
	}
	if err := Validate(spec.InputSchema, args); err != nil {
		return tools.Result{}, fmt.Errorf("%s: %w", name, err)
	}
	sem := d.semaphore(name)
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return tools.Result{}, ctx.Err()
	}
	defer func() { <-sem }()

	runCtx := ctx
	cancel := func() {}
	if d.cfg.CallTimeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, d.cfg.CallTimeout)
	}
	defer cancel()

	label := d.Egress(name)
	if d.cfg.OnCall != nil {
		d.cfg.OnCall(name, label)
	}
	return d.exec.Execute(runCtx, model.ToolCall{Name: name, Input: args})
}

func (d *Dispatcher) Egress(name string) string {
	if label, ok := d.cfg.Egress[name]; ok && label != "" {
		return label
	}
	if spec, ok := d.exec.Spec(name); ok && (spec.Risk == tools.RiskWrite) {
		return EgressLocal
	}
	return EgressNone
}

func (d *Dispatcher) semaphore(name string) chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	if sem, ok := d.sems[name]; ok {
		return sem
	}
	limit := d.cfg.DefaultConcurrency
	if n, ok := d.cfg.Concurrency[name]; ok && n > 0 {
		limit = n
	}
	sem := make(chan struct{}, limit)
	d.sems[name] = sem
	return sem
}
