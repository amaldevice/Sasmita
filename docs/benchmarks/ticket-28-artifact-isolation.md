# Ticket #28 — sandboxed artifacts: what is verified, what is not

Date: 26 Nov 2026. Branch: `ticket/28-sandboxed-artifacts`. All checks ran under
the shared build lock (8 GB / 6 CPU).

## Verified locally (seams 1 and 2)

- **Seam 1 (Go):** `internal/artifacts` tests cover `create_artifact`,
  `edit_artifact` (0 matches and >1 matches rejected with an `EditError` meant
  for the model), `rewrite_artifact`, immutable version append, the 512 KB
  source cap, the 5 MB per-data-ref cap (declared and resolver-backed), esbuild
  JSX transpile, `artifact://` CSP header, read-only handler (POST → 405),
  import map with all eight pinned libraries, clear failure for unknown
  runtime imports, `froze_view` never auto-runs, main-frame navigation guard,
  atomic file writes, migration `013` Tier-2 columns, DB mirror, and reuse of
  #22's tool registry/result path.
- **Seam 2 (Playwright):** the library lists every artifact with its versions
  and runtime, the iframe has `sandbox="allow-scripts"` only (no `srcdoc`, no
  `allow-same-origin`), bridge v1 accepts `resize`, `reportError`,
  `saveState`/`loadState` (32 KB cap), `getData(ref)`, `suggestPrompt`, and
  rejects wrong origin, wrong `event.source`, unknown types/channels and
  oversized state. Streaming source is visible before storage and the iframe
  gets no `artifact://` src until the artifact is stored. A main-frame link to
  `artifact://` is neutralised.
- `go test ./... -count=1`: all packages ok. `npm run check`: 0 errors.
  `npm run test:e2e`: 25 passed (6 new artifact specs).

## NOT verified locally — packaged-app / S1 isolation gate

The S1 gate (spike plan §"isolation absolute") requires a **packaged MyGo
WKWebView** run: from a sandbox `artifact://` iframe confirm `window.mygo` is
undefined, subframe `postMessage` never reaches Go, CSP `sandbox`/`connect-src
'none'` is honoured, external navigation and popups fail, and `while(true){}`
recovers through the watchdog within ~5 s. **None of this was measured here** —
this environment has no packaged MyGo shell to drive. These remain manual /
packaged checks; we do not claim WKWebView isolation is verified.

No failure was observed, so the pre-decided fallback (hold Tier 2 inline only,
or use an `Untrusted` window if iframe messages reach Go) was **not triggered**.
The implementation follows ADR-0004 and `research/mygo-isolation.md` so the
fallback stays a configuration change, not a rewrite: the Go handler has no
side effects, all actions go through the v1 bridge whitelist, and main-frame
navigation is rejected in both Go and the frontend.

Packaged check to run before shipping:
1. Build the MyGo app and open two inline artifacts plus one WebGL artifact.
2. From each iframe, attempt `window.mygo`, `webkit.messageHandlers.mygo`,
   `fetch('mygo://localhost/')`, `fetch('https://…')`, `top.location=…`,
   `window.open`, and a form submit; log Go-side IPC to prove nothing arrives.
3. Confirm `origin === "null"` and that CSP is applied to the served document.
4. Freeze the main thread for ~3 s and verify watchdog reload + `froze_view`
   with no auto-run; measure process-tree RSS for 0/1/2/5/20 iframes.

## Runtime bundles

`artifact-runtime/1` is served locally through the import map — all eight
specifiers resolve to `artifact://runtime/1/…` with no CDN fallback, and
unknown imports return a clear 404. The modules in this build are
version-pinned ESM shims: they expose the expected exports but throw an
explicit "bundle is not installed in this build" error when invoked. Installing
the full third-party bundles (React, ReactDOM, d3, Chart.js, lucide, Tailwind
browser runtime, three.js + OrbitControls) is a packaging step tied to the S1
packaged check above. This is recorded here rather than implied to be complete.
