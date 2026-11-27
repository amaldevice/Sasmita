package main

import (
	"context"
	"errors"
	"log"
	"sync/atomic"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/watchdog"
	"github.com/egoist/mygo"
)

// pageProbe pings the trusted page. EvalContext is bounded because a hung main
// thread cannot answer; a timeout is a miss, not an application error.
type pageProbe struct {
	page    *mygo.Page
	timeout time.Duration
}

func (p pageProbe) Alive(ctx context.Context) (bool, error) {
	callCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	value, err := p.page.EvalContext(callCtx, "1")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return false, nil
		}
		return false, err
	}
	return value != nil, nil
}

// liveReporter holds the artifact IDs the renderer last reported as live.
type liveReporter struct{ ids atomic.Value }

func (r *liveReporter) Set(ids []string) { r.ids.Store(append([]string(nil), ids...)) }

func (r *liveReporter) LiveArtifacts(context.Context) []string {
	ids, _ := r.ids.Load().([]string)
	return append([]string(nil), ids...)
}

// ReportLiveArtifacts is a one-way frontend → Go binding. It never returns a
// value to the sandboxed artifact; only the trusted renderer can call it.
func (s *shellService) ReportLiveArtifacts(_ context.Context, ids []string) {
	s.live.Set(ids)
}

type frozeMarker struct{ app *appsvc.Service }

func (m frozeMarker) MarkFroze(ctx context.Context, id string) error {
	return m.app.MarkArtifactFroze(ctx, id)
}

type pageReloader struct{ page *mygo.Page }

func (r pageReloader) Reload(context.Context) error {
	// Reload destroys every live iframe; the renderer reallocates slots when
	// the page comes back and never auto-runs the marked offender.
	r.page.Reload()
	return nil
}

func (s *shellService) OpenArtifactWindow(_ context.Context, uri string) error {
	if uri == "" {
		return errors.New("artifact URI is required")
	}
	// Untrusted is per window: no Go calls, no broad navigation, own process.
	mygo.NewWindow(mygo.WindowOptions{Title: "Artifact", URL: uri, Page: mygo.PageOptions{Untrusted: true}})
	return nil
}

func startWatchdog(ctx context.Context, page *mygo.Page, app *appsvc.Service, live *liveReporter) {
	detector := watchdog.New(
		pageProbe{page: page, timeout: watchdog.DefaultTimeout},
		live, frozeMarker{app: app}, pageReloader{page: page},
		watchdog.Config{Timeout: watchdog.DefaultTimeout},
	)
	go func() {
		report, err := detector.Watch(ctx)
		if err != nil {
			return
		}
		if report.Kind == watchdog.FrozeView {
			log.Printf("watchdog: %s froze the view; page reloaded", report.ArtifactID)
		} else {
			log.Printf("watchdog: main thread unresponsive (%s)", report.Kind)
		}
	}()
}
