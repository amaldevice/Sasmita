package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/runtime"
)

// The run manager is the seam between a UI cancellation command and the
// contexts handed to the provider and local work (TAD §14.3).
func TestManagerCancelPropagatesAndForgetsFinishedRuns(t *testing.T) {
	m := runtime.NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Register("r1", cancel, nil)

	if err := m.Cancel("r1"); err != nil {
		t.Fatalf("Cancel(r1) = %v", err)
	}
	select {
	case <-ctx.Done():
	case <-t.Context().Done():
		t.Fatal("cancel did not reach the run context")
	}
	if err := m.Cancel("missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Cancel(missing) = %v, want ErrNotFound", err)
	}

	m.Finish("r1")
	if err := m.Cancel("r1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Cancel(finished) = %v, want ErrNotFound", err)
	}
}
