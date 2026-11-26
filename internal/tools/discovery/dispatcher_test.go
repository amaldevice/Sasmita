package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/tools"
)

type fakeExecutor struct {
	specs   map[string]tools.Spec
	execute func(context.Context, model.ToolCall) (tools.Result, error)
}

func (f fakeExecutor) Spec(name string) (tools.Spec, bool) {
	spec, ok := f.specs[name]
	return spec, ok
}

func (f fakeExecutor) Execute(ctx context.Context, call model.ToolCall) (tools.Result, error) {
	return f.execute(ctx, call)
}

func countingExecutor() fakeExecutor {
	return fakeExecutor{
		specs: map[string]tools.Spec{
			"slow": {Name: "slow", InputSchema: json.RawMessage(`{"type":"object"}`), Risk: tools.RiskCompute},
			"edit": {Name: "edit", InputSchema: json.RawMessage(`{"type":"object"}`), Risk: tools.RiskWrite},
		},
	}
}

// Per-tool limits bound concurrent execution; the semaphore is never shared
// across tool IDs so a slow tool cannot starve an unrelated one.
func TestDispatcherPerToolConcurrencyLimit(t *testing.T) {
	exec := countingExecutor()
	var active, peak atomic.Int32
	exec.execute = func(ctx context.Context, _ model.ToolCall) (tools.Result, error) {
		now := active.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		<-ctx.Done()
		active.Add(-1)
		return tools.Result{}, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	d := NewDispatcher(exec, DispatcherConfig{
		Concurrency: map[string]int{"slow": 2},
		CallTimeout: 40 * time.Millisecond,
	})
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = d.Call(ctx, "slow", json.RawMessage(`{}`))
		}()
	}
	wg.Wait()
	if got := peak.Load(); got != 2 {
		t.Fatalf("peak concurrency = %d, want 2", got)
	}
}

// A hard call timeout bounds even a tool that ignores its own deadline.
func TestDispatcherCallTimeout(t *testing.T) {
	exec := countingExecutor()
	exec.execute = func(ctx context.Context, _ model.ToolCall) (tools.Result, error) {
		select {
		case <-ctx.Done():
			return tools.Result{}, ctx.Err()
		case <-time.After(2 * time.Second):
			return tools.Result{Output: "late"}, nil
		}
	}
	d := NewDispatcher(exec, DispatcherConfig{CallTimeout: 20 * time.Millisecond})
	_, err := d.Call(context.Background(), "slow", json.RawMessage(`{}`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}

// Egress labels are attached at dispatch time so a future run_script can reuse
// the same boundary without a second authorization path.
func TestDispatcherEgressLabels(t *testing.T) {
	exec := countingExecutor()
	var labels []string
	exec.execute = func(context.Context, model.ToolCall) (tools.Result, error) {
		return tools.Result{Output: "ok"}, nil
	}
	d := NewDispatcher(exec, DispatcherConfig{
		Egress: map[string]string{"slow": EgressNetwork},
		OnCall: func(_, label string) { labels = append(labels, label) },
	})
	if _, err := d.Call(context.Background(), "slow", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Call(context.Background(), "edit", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if len(labels) != 2 || labels[0] != EgressNetwork || labels[1] != EgressLocal {
		t.Fatalf("egress labels = %v", labels)
	}
}
