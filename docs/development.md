# Development gaps

Current state of Sasmita against [TAD v0.3](../SASMITA_TAD_v0.3.0.md), as of `v0.1.0-alpha.1`. TAD owns the requirements; this file owns the **gap**: current state and the observable "done when" bound for closing it. Shipped behaviour lives in [FEATURE.md](../FEATURE.md). Windows, Linux, auto-update, and crash reporting are tracked in issue #14 and intentionally absent here.

## Trigger → section

| Trigger | Section |
|---|---|
| Implement a deferred capability | Phase 4, Phase 5, Phase 6 |
| Close the first public beta | Beta gates |
| Run or evaluate a spike | Gate evidence |
| Settle product scope | Open decisions |

## Phase 4 — integration and computation

Phase 4 also lists approval/resume and Keychain. Both shipped in v1 and sit outside this gap.

### Analis Data (`query_table` + Pyodide)

- **Requirement:** `query_table` is the first path for CSV/JSON/tabular data, returning schema, row count, aggregates, and a small sample with paging (TAD §11.6). Python is a fallback in a hidden `Untrusted` window: WebContent process + WASM, `connect-src 'none'`, files copied as bytes, no host filesystem per run, and code visible in an expandable panel (TAD §5, §20). Gate: cold start ≤3 s, idle shutdown after 10 min, timeout 60 s, input 200 MB, ~0.4–0.55 GB while pandas is loaded, no `fetch`, OOM isolation.
- **Current:** `query_table` is deliberately excluded from the Mode catalog (`internal/modes/catalog.go`, `internal/modes/catalog_parity_test.go`); the Mode is listed as unavailable; the PDF/DOCX/XLSX parsing library is undecided.
- **Done when:** a 50 MB workbook query returns bounded schema, sample, and paging through `query_table`; a Python fallback runs in the hidden Untrusted window with no network and no host filesystem; and the S4 gate is recorded.

### MCP

- **Requirement:** official Go MCP SDK behind `ToolRegistry` and `MCPManager` (TAD §10.3). Add/edit/remove servers, enable/disable, and test connection. STDIO and Streamable HTTP transports. Discover tools only after explicit connection; bind each tool to a permission profile and output/time limits; terminate local processes on disable or exit; apply startup/call/idle timeouts, concurrency caps, and cancellation; treat descriptions and results as data; require explicit consent before sending private data; show connection status and actionable errors; keep secrets in the OS credential store with only references in SQLite. Tables: `mcp_servers`, `mcp_tool_cache`.
- **Current:** no MCP tables in migrations and no SDK integration. `find_tools`, `call_tool`, the dispatcher, and the policy broker are already in place, so this is a plug-in rather than a rewrite.
- **Done when:** the add → test → enable → disable → disconnect cycle passes for both transports against fake servers; secrets are absent from SQLite; server processes terminate on disable and exit; unknown servers stay disabled.

### Custom Modes and permission scopes

- **Requirement:** user-defined Modes with an explicit tool set, scope, and policy, alongside the built-ins (TAD §10, §22 Phase 4).
- **Current:** five built-ins; Chat, Riset, and Penulisan are selectable, Analis Data and Coding are listed as coming later. Policy covers built-in scopes only.
- **Done when:** a user-created Mode with an explicit tool set and scope is selectable, and tools outside that set are denied by policy.

## Phase 5 — long-form and coding

### Long-form generation

- **Requirement:** a generation task has its own ID, plan, section states, token/cost budget, and artifact IDs (TAD §12). Each section is persisted when complete; active context holds the outline, local section requirements, a continuity summary, and selected references instead of every previous paragraph. Provide pause/resume, cancel, regenerate section, rewrite selection, continue, and export. Store versions rather than overwriting approved work; clamp budgets to provider capability and safety limits; continue from partial output only through a controlled flow.
- **Current:** no generation task workflow. The DoD item "long-form job can pause, resume, and preserve completed sections" is unchecked.
- **Done when:** a long report interrupted mid-run restores completed sections, resumes at the next queued section, and exports a versioned artifact without overwriting approved versions.

### Mode Coding and Seatbelt shell

- **Requirement:** structured operations preferred over raw commands. When an executable is invoked: fixed path or allowlist, argument array, clean environment, working-directory scope, timeout, output cap, and cancellation (TAD §15.4). The shell is opt-in inside Mode Coding under `/usr/bin/sandbox-exec` with `(deny default)`, writes limited to workspace and temp, `.git` read-only, network off, and per-command allow/ask/deny (TAD §5, ADR-0005).
- **Current:** no shell. ADR-0005 keeps it disabled; there is no unsandboxed fallback.
- **Done when:** the Seatbelt allow/deny cases pass with network and out-of-workspace writes denied, and the Mode remains disabled when `sandbox-exec` is unavailable.

## Phase 6 — advanced capabilities

### Code Mode `run_script`

- **Requirement:** a fixed `run_script` tool executes model-written JS in QuickJS-WASM with an empty filesystem; host functions go through the tool dispatcher only (TAD §10.4). Per-tool concurrency, per-call timeout, script deadline and output cap, per-call policy with batched approvals, and egress labels. The tool description never contains the tool catalog. Off by default and enabled only for models that pass S3.
- **Current:** the dispatcher is designed for reuse by `run_script`; the tool itself is absent.
- **Done when:** a script calling N tools respects concurrency caps, deadline, output cap, batched approvals, and egress labels; filesystem and network are unreachable; the tool stays off for models that fail S3.

### MCP Apps

- **Requirement:** server-provided UI rendered as a third-party trust domain in an isolated frame with a narrow message bridge (TAD §12A.10). No host DOM, app state, cookies, Keychain, filesystem, MyGo IPC, or native APIs. Network deny by default. Validate origin, source window, message schema, and resource identity. No top navigation, popups, clipboard, downloads, or external links without explicit policy. UI-to-tool requests pass through the normal broker. Deferred until the WebView isolation and bridge threat-model spike passes.
- **Current:** disabled; no isolation spike has passed.
- **Done when:** a packaged-app test proves no frame message reaches Go, CSP and network policy hold, and the isolation spike is recorded. Until then the capability stays disabled.

### AI-powered artifacts

- **Requirement:** a bounded write → validate → reject with reason → revise loop for model-written artifacts, preserving previous versions.
- **Current:** artifact create/edit/rewrite exists; the generative revision loop does not.
- **Done when:** an invalid model-written artifact is rejected with a reason and revised within a bounded attempt limit, with earlier versions intact.

### Vector retrieval and memory

- **Requirement:** FTS first; embeddings only when an evaluation shows a meaningful improvement over FTS (TAD §16). Retrieval is optional, lazy, and process-independent when resource costs warrant it. Every item carries provenance, timestamp, project scope, and relevance rationale. Table: `memory_items`.
- **Current:** SQLite FTS conversation search ships; `memory_items` and embeddings do not exist.
- **Done when:** an evaluation records a measurable improvement over FTS, and optional retrieval returns scoped items with provenance, timestamp, and relevance rationale.

### Multi-agent

- **Requirement:** multi-agent orchestration remains optional. Launching a swarm for every request is an explicit non-goal (TAD §6).
- **Current:** no orchestration layer.
- **Done when:** an opt-in multi-agent run is bounded by a task budget and the default path stays single-agent.

## Cross-cutting gaps

### Backup, restore, and data lifecycle

- **Requirement:** consistent snapshot of database and artifact files, user-facing export and deletion, retention controls, and no automatic deletion of session history (TAD §13). Encryption at rest is evaluated only after backup/restore exists.
- **Current:** WAL and versioned migrations ship; there is no snapshot, export, deletion, or retention control.
- **Done when:** a restore of a snapshot taken before a migration reproduces conversations, approvals, and artifacts; export and deletion are reachable in Settings; history is never deleted automatically.

### Native tray and menu

- **Requirement:** the MyGo spike must prove tray and menu alongside IPC, window lifecycle, file dialogs, permissions, and packaging (TAD §4).
- **Current:** file dialogs and native notifications are wired; tray and menu are not.
- **Done when:** tray and menu actions are wired through `internal/desktop` and exercised by the shell seam.

## Gate evidence

Gate definitions live in [spike-plan.md](spike-plan.md); automated coverage is summarised in [FEATURE.md](../FEATURE.md). The table records **status only**.

| Gate | Status |
|---|---|
| Golden prefix, both adapters | Closed by seam-1 tests |
| ≥80% cache hit from turn 3 on DeepSeek/OpenRouter | Open — no live credentials; see [S2](benchmarks/s2-adapters-cache.md) |
| RSS idle ≤150 MB, chat ≤250 MB, startup ≤2 s | Open — see [packaged build](benchmarks/package-build-final.md) |
| WKWebView CSP and bridge isolation | Open — see [ticket 28](benchmarks/ticket-28-artifact-isolation.md) |
| Freeze detection and UI recovery ≤5 s in packaged app | Open — see [ticket 29](benchmarks/ticket-29-live-artifacts.md) |
| `find_tools`/`call_tool` ≥90%, visual-first ≥90% | Open — S3 not run |
| Pyodide cold start ≤3 s, idle shutdown, OOM isolation | Open — Phase 4 |
| Seatbelt allow/deny cases | Open — Phase 5 |
| MCP add/test/enable/disable/disconnect | Open — Phase 4 |
| Long-form pause/resume/sections | Open — Phase 5 |
| Prompt injection, output bombs, MCP responses, GenUI XSS, cross-origin MCP App injection | Open — TAD §20; path traversal, approval binding, cancellation, and restart are already covered |

## Open decisions

| Decision | Recorded when |
|---|---|
| Default provider for users without an API key | An ADR records the chosen default or the explicit bring-your-own policy |
| Web search provider | An ADR records the provider and its egress disclosure |
| PDF/DOCX/XLSX parsing library | An ADR records the library and its licence/distribution impact |
| `run_script` threshold per model | S3 defines the threshold and the enablement policy |
| "Limited support" model marking | Models failing S3 lose meta-tools and visual-first in the UI |
| Encryption at rest (SQLCipher) | Evaluated after backup/restore; an ADR records the decision |
| A2UI / AG-UI adapters | A second frontend or interoperability requirement is recorded; until then `sasmita.ui/v1` is the only contract |
