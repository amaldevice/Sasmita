# Features

This is the inventory of Sasmita's current early-preview build. It describes shipped behaviour, not planned work. Release notes live on [GitHub Releases](https://github.com/amaldevice/Sasmita/releases/latest); design rationale lives in [SASMITA_TAD_v0.3.0.md](SASMITA_TAD_v0.3.0.md).

## Conversations

- Streaming answers with cancel, persisted locally, and recovered as interrupted after an app restart.
- Conversation search by title and message text (SQLite FTS5).
- Rename conversations and move them into projects.
- Projects define the folder roots available to file tools. A conversation without a project sees only its attachments; no folder access is granted implicitly.
- Native file picker: attach files as chips, scoped to the message and run.
- Sidebar with New chat, Search, Projects, Artifacts, and recent conversations; collapse to a 64 px rail with the button or ⌘\\.
- A run waiting or finishing in another conversation shows a sidebar badge; macOS notifications fire only while the app is not frontmost.

## Modes and providers

- Modes available now: Chat, Riset, Penulisan. Analis Data and Coding are listed as coming later and cannot be selected.
- Multiple provider profiles: OpenAI-compatible, Anthropic-compatible, and local endpoints. Each stores a base URL and a Keychain-backed secret reference.
- Test connection returns plain-language results; model capability (tools, vision, context size) is shown as known or unknown.
- Mode and model chips in the composer. Switching past roughly 4,000 tokens warns about the re-read cost; switching providers preserves canonical history while dropping provider-specific content blocks.
- First run offers detection of local Ollama and llama.cpp endpoints and a **Use an API** path. Detected endpoints are never presented as already tested, and four example prompts seed an empty chat.

## Agent tools and approvals

- Built-in tools: calculator, project/attachment file reader, bounded search over conversations and files, artifact writer, and artifact create/edit/rewrite.
- Discovery meta-tools: `find_tools` ranks enabled tools from a local index under a size cap; `call_tool` validates the real tool's schema before execution; `load_skill` returns skill content as a tool result.
- Each tool carries a risk level, timeout, output cap, and egress label; file roots come from the conversation's project and attachments.
- One-line activity summary above each answer expands into a timeline of steps, distinguishing proposed from executed tool calls.
- Approvals offer Allow once, Always allow in this project, and Deny. A decision is bound to the exact arguments and policy snapshot, and is revalidated at resolve time and on resume after restart.
- Cancellation stops a streaming run; an interrupted run offers a Continue action.

## Visual answers

- Visual-first order: a UI block or artifact streams first, then the paragraph explanation. Simple answers remain text-only.
- UI blocks: chart, table, Mermaid diagram, KPI view, and math, using Sasmita's own components. Sliders, tabs, and toggles update locally without a model call; invalid specs fall back to text with a short reason and re-render from stored spec after reopen.
- Sandboxed artifacts: immutable versions with create/edit/rewrite, served from an `artifact://` handler with `sandbox allow-scripts; default-src 'none'; connect-src 'none'`. The frontend bridge accepts only a whitelisted message set: resize, error report, save/load state, bounded data requests, and suggested prompt.
- Artifact lifecycle: up to two live artifacts and one WebGL artifact. They auto-run when visible, park as cards when off-screen, and show a frozen state with Retry, Open in separate window, and View code if they hang. State is preserved across park and restart within a 32 KB cap.
- Artifacts library lists saved artifacts; an artifact can open beside the chat and go full screen, with Esc to exit.
- Answer formats: one column, visual on top with two text columns below, or side by side. Chat and Baca (reading) views differ in rail and column width.

## Context, cost, and persistence

- Requests are built append-only with deterministic JSON and a stable session key so provider prompt caching keeps hitting.
- Automatic compaction near the context budget plus a manual control in Settings; canonical history is preserved and recent turns stay verbatim.
- Per-answer usage popover shows input/output tokens, cache-hit rate, and estimated cost. An optional setting keeps the usage line visible. Diagnostics aggregates usage per model and per day and flags cache-rate drops.
- SQLite with WAL and versioned migrations stores conversations, runs, approvals, artifacts, and usage. History, artifacts, and settings remain on the device; `SASMITA_DB_PATH` relocates the database.

## Platform and accessibility

- macOS 12 or later on Apple Silicon, distributed as an ad-hoc signed DMG.
- Full Indonesian and English copy, dark default theme with a light option, and reduced-motion behaviour.
- API keys are stored in the macOS Keychain; only secret references appear in SQLite. Remote requests are marked in the UI, and risky tool calls require approval.

## Verification status

- Covered by automated checks: Go unit and seam-1 integration tests, 51 Playwright seam-2 tests, `svelte-check`, frontend build, and an arm64 cross-compile in CI.
- Still manual on the packaged app: WKWebView enforcement of the artifact CSP and bridge isolation, real freeze detection and reload recovery, per-iframe memory, idle/chat RSS, and startup time. Evidence and remaining checks are in `docs/benchmarks/`.
- Not in this build: MCP servers, Data Analyst Mode with spreadsheet/PDF parsing, long-form generation, Windows/Linux, auto-update, and Developer ID notarization.
