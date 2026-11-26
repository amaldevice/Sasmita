// Package watchdog watches the UI main thread for hangs. It owns no MyGo type:
// the shell supplies a probe that asks the page whether it is alive, plus the
// callbacks for recording the offending artifact and reloading the page.
package watchdog

import (
	"context"
	"errors"
	"time"
)

// DefaultTimeout is the TAD §12A.15 hang threshold (~3 s).
const DefaultTimeout = 3 * time.Second

const defaultInterval = 500 * time.Millisecond

// Probe reports whether the frontend main thread answered the last ping.
type Probe interface {
	Alive(ctx context.Context) (bool, error)
}

// LiveReporter names the artifacts currently holding inline iframe slots.
type LiveReporter interface {
	LiveArtifacts(ctx context.Context) []string
}

type Marker interface {
	MarkFroze(ctx context.Context, id string) error
}

type Reloader interface {
	Reload(ctx context.Context) error
}

type Config struct {
	Timeout  time.Duration
	Interval time.Duration
}

type Report struct {
	Kind       string
	ArtifactID string
	Err        error
}

const (
	FrozeView      = "froze_view"
	NoLiveArtifact = "no_live_artifact"
)

type Detector struct {
	probe    Probe
	live     LiveReporter
	marker   Marker
	reloader Reloader
	timeout  time.Duration
	interval time.Duration
}

func New(probe Probe, live LiveReporter, marker Marker, reloader Reloader, cfg Config) *Detector {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.Interval <= 0 {
		cfg.Interval = defaultInterval
	}
	if cfg.Interval > cfg.Timeout {
		cfg.Interval = cfg.Timeout
	}
	return &Detector{probe: probe, live: live, marker: marker, reloader: reloader, timeout: cfg.Timeout, interval: cfg.Interval}
}

// Watch runs until the context ends or a hang is confirmed. A single missed
// ping is not a freeze: the unresponsive state must persist for the timeout.
func (d *Detector) Watch(ctx context.Context) (Report, error) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	var unresponsiveSince time.Time
	for {
		select {
		case <-ctx.Done():
			return Report{}, ctx.Err()
		case <-ticker.C:
			alive, err := d.probe.Alive(ctx)
			if err == nil && alive {
				unresponsiveSince = time.Time{}
				continue
			}
			now := time.Now()
			if unresponsiveSince.IsZero() {
				unresponsiveSince = now
				continue
			}
			if now.Sub(unresponsiveSince) < d.timeout {
				continue
			}
			return d.escalate(ctx, err)
		}
	}
}

// escalate records the live offender first, then reloads so inline iframes are
// destroyed and slots are reallocated. A reload without an offender is not
// attempted: there is nothing safe to mark and no reason to lose UI state.
func (d *Detector) escalate(ctx context.Context, cause error) (Report, error) {
	ids := d.live.LiveArtifacts(ctx)
	if len(ids) == 0 {
		return Report{Kind: NoLiveArtifact, Err: cause}, nil
	}
	id := ids[0]
	if err := d.marker.MarkFroze(ctx, id); err != nil {
		return Report{Kind: FrozeView, ArtifactID: id, Err: err}, err
	}
	if err := d.reloader.Reload(ctx); err != nil {
		return Report{Kind: FrozeView, ArtifactID: id, Err: err}, err
	}
	return Report{Kind: FrozeView, ArtifactID: id, Err: cause}, nil
}

// IsHang reports whether a probe error is a main-thread hang rather than a
// cancelled context. The shell uses it to avoid escalating during shutdown.
func IsHang(err error) bool {
	return err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}
