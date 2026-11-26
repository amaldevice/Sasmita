# S1 sample: MyGo walking skeleton (ticket #15)

Date: 26 Nov 2026. Machine: Apple A18 Pro, 8 GB RAM, macOS 27.2 (26B123), darwin/arm64.
These are **single samples**, not the full S1 gate from `docs/spike-plan.md`.

## What was actually verified

- `go build ./...` in `shell/mygo/` succeeds with Go 1.27.3 (fetched automatically
  by `GOTOOLCHAIN=auto`; the local Go was 1.27.0). MyGo is pinned to `v0.10.0`.
- `mygo generate` produced `frontend/src/mygo.ts` with typed bindings for all
  five commands, including `StartRun` returning the run ID and streaming
  `RunEvent` through a typed `Channel`. The frontend `MyGoIPC` uses that client.
- `mygo build` produced `build/darwin-arm64/Sasmita.app` (14.5 MB) and
  `sasmita-0.1.0-arm64.dmg` (4.4 MB), both ad-hoc signed by MyGo.
- The packaged `.app` launched via `open` with `SASMITA_OPENAI_BASE_URL` and
  `SASMITA_DB_PATH` set; the process stayed alive and the WebView process was
  created. No notarization or Gatekeeper bypass was attempted.

## RSS samples (aggregate process tree, `ps -Ao pid,ppid,rss,comm`)

| State | Sasmita.app process | WebContent process | Sum |
|---|---|---|---|
| Idle, empty conversation | 58,032 KB | 25,328 KB | 83,360 KB (~81 MB) |
| Chat, 8 messages pre-seeded in SQLite | 31,872 KB | 10,256 KB | 42,128 KB (~41 MB) |

Commands: launch with `SASMITA_OPENAI_BASE_URL=http://127.0.0.1:9
SASMITA_DB_PATH=<temp>/sasmita-bench.db open build/darwin-arm64/Sasmita.app`,
wait 6 s, then `ps`. The chat sample used a conversation seeded with eight
messages so the WebView rendered a realistic history.

Caveats, stated plainly:

- Single sample per state; the idle figure includes first-launch warm-up while
  the chat figure was a second launch. The direction of the difference is not
  meaningful — only that both are far below the 150/250 MB targets.
- WebContent is a shared system XPC service; attributing a given PID to this
  app is inferential, so the sum is an estimate of the aggregate tree.
- "Chat" RSS was measured with a pre-seeded history, **not** during a live
  streamed provider turn. The chat streaming path is covered by the two test
  seams, not by this packaged-app measurement.
- Startup time, goroutine stability, and repeated-sample statistics were not
  measured.

## Not verified (S1 gate remains open)

The full S1 isolation checklist (bridge main-frame-only, CSP headers, `srcdoc`,
`Origin`/`CallerPage`, navigation, workers, per-iframe memory, `while(true)`
recovery, `Untrusted` window, COOP/COEP, three.js, snapshot capture), watchdog
recovery, and Pyodide were not exercised. Packaged IPC streaming against a real
OpenAI-compatible endpoint was not driven through the GUI. Therefore S1 is
**not** declared passed. The pre-decided fallback stays in place: if MyGo fails
a fundamental gate (IPC, packaging, memory), re-evaluate Wails v3. The swap is
contained — all application calls go through `internal/desktop.Bindings`, and
the shell lives in the separate `shell/mygo` module.
