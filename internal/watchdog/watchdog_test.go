package watchdog_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/watchdog"
)

type fakeProbe struct {
	mu     sync.Mutex
	alive  []bool
	errors []error
	calls  int
}

func (p *fakeProbe) Alive(context.Context) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := p.calls
	p.calls++
	if i < len(p.errors) && p.errors[i] != nil {
		return false, p.errors[i]
	}
	if i < len(p.alive) {
		return p.alive[i], nil
	}
	if len(p.alive) > 0 {
		return p.alive[len(p.alive)-1], nil
	}
	return true, nil
}

type recorder struct {
	mu     sync.Mutex
	live   []string
	marked []string
	loads  int
}

func (r *recorder) LiveArtifacts(context.Context) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.live...)
}
func (r *recorder) MarkFroze(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.marked = append(r.marked, id)
	return nil
}
func (r *recorder) Reload(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loads++
	return nil
}

func TestDefaultTimeoutIsAroundThreeSeconds(t *testing.T) {
	if watchdog.DefaultTimeout != 3*time.Second {
		t.Fatalf("DefaultTimeout = %s, want 3s", watchdog.DefaultTimeout)
	}
}

func TestDetectorIgnoresResponsiveMainThread(t *testing.T) {
	probe := &fakeProbe{alive: []bool{true, true, true, true}}
	rec := &recorder{live: []string{"a"}}
	d := watchdog.New(probe, rec, rec, rec, watchdog.Config{Timeout: 20 * time.Millisecond, Interval: time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if _, err := d.Watch(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Watch returned %v, want the context to stay healthy until timeout", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.marked) != 0 || rec.loads != 0 {
		t.Fatalf("a responsive thread was reloaded: marked=%v loads=%d", rec.marked, rec.loads)
	}
}

func TestDetectorMarksOffenderAndReloadsAfterTimeout(t *testing.T) {
	probe := &fakeProbe{alive: []bool{false}}
	rec := &recorder{live: []string{"webgl-a", "plain-b"}}
	d := watchdog.New(probe, rec, rec, rec, watchdog.Config{Timeout: 40 * time.Millisecond, Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	report, err := d.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if report.Kind != watchdog.FrozeView || report.ArtifactID != "webgl-a" {
		t.Fatalf("report = %+v, want froze_view / webgl-a", report)
	}
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond || elapsed > 400*time.Millisecond {
		t.Fatalf("detection took %s, want close to the configured timeout", elapsed)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.marked) != 1 || rec.marked[0] != "webgl-a" || rec.loads != 1 {
		t.Fatalf("after freeze marked=%v loads=%d", rec.marked, rec.loads)
	}
}

func TestDetectorReportsErrorsWithoutMarkingWhenNoArtifactIsLive(t *testing.T) {
	probe := &fakeProbe{alive: []bool{false}}
	rec := &recorder{}
	d := watchdog.New(probe, rec, rec, rec, watchdog.Config{Timeout: 10 * time.Millisecond, Interval: time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	report, err := d.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Kind != watchdog.NoLiveArtifact || report.ArtifactID != "" {
		t.Fatalf("report = %+v", report)
	}
	if rec.loads != 0 {
		t.Fatal("page was reloaded with no live artifact to blame")
	}
}
