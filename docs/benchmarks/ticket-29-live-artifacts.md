# Ticket #29 — live artifact lifecycle and watchdog: what is verified

Date: 26 Nov 2026. All checks ran under the shared build lock (8 GB / 6 CPU).

## Verified locally

- **Seam 2 (Playwright, `frontend/tests/artifact-lifecycle.spec.ts`):** slot
  allocation follows the prototype `RenderFlow` order exactly (expanded, then
  last user Run, then most recent visible); at most two live artifacts and one
  WebGL, with `slots_full` cards for visible losers. Off-screen blocks call the
  same `saveState` storage as bridge v1, lose the iframe and show a card with
  title, version, summary and Run; Run restores the saved state and reallocates.
  Reopening a conversation reallocates only the last two visible artifacts, and
  a block that was streaming becomes `interrupted` with "Ask the model to
  continue". Watchdog callback marks the offender `froze_view`, frees its slot,
  and the card offers Retry / Open in separate window / View code. The separate
  window hook receives `{ untrusted: true }`.
- **Go unit tests (`internal/watchdog`):** a responsive main thread is never
  reloaded; an unresponsive one is escalated only after the configured timeout,
  marks the first live artifact and calls reload once; no live artifact means no
  reload. The default threshold is 3 s.
- The shell module compiles with the MyGo adapter: `EvalContext` probe, live-ID
  binding, `MarkArtifactFroze`, `Page.Reload`, and an `Untrusted` window for
  "Open in separate window".
- `go test ./... -count=1`: all packages ok. `npm run check`: 0 errors/0 warnings.
  `npm run test:e2e`: all specs pass.

## NOT verified locally — packaged-app checks

No real MyGo/WKWebView app was built here, so **real freeze detection, reload
recovery and memory per iframe are not measured**. The seam-2 watchdog test
invokes the same callback the Go detector would call; it does not hang a real
main thread. The `Untrusted` flag is passed to `mygo.NewWindow` but the separate
WebContent process and its overhead were not observed. These stay manual checks:

1. Build the packaged app, open two inline artifacts plus one WebGL, and
   measure aggregate process-tree RSS (0/1/2/5/20 iframes) against the
   ≤ +150 MB target.
2. Run `while(true){}` in a live artifact; confirm the detector reloads within
   ~3 s, the offender is `froze_view`, slots reallocate and chat recovers.
3. Open a frozen artifact in the separate window; confirm `window.mygo` is
   unavailable there and that a hang in it does not freeze chat.

The pre-decided S1 fallback was **not triggered** by any local evidence: nothing
observed iframe messages reaching Go or the watchdog failing. If the packaged
check shows either, Tier 2 stays inline only in the `Untrusted` window rather
than being declared safe; that is a configuration change in the shell, not a
rewrite of the allocator.
