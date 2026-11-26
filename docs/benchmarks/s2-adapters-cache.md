# S2 — adapters + prompt cache (ticket #17)

## Gate status — honest

The S2 gate (`docs/spike-plan.md:71`) needs a real 30+ turn session on DeepSeek,
OpenRouter and Claude with ≥80% cache hit from turn 3. This machine has no provider
credentials or live endpoints, so **the hit-rate gate was not measured**. No number
is claimed here. The pre-decided fallback from `docs/spike-plan.md:82` was applied
proactively: *fix the adapters, do not change the architecture*. Only fake
`httptest` providers were used; they verify request-byte invariants, not provider
behavior.

## The breakpoint / golden-prefix tension

A 5m breakpoint must sit on the **last content block**. On the next turn that
block is no longer last, so moving the marker rewrites bytes of a previously
sent request. A full-wire golden prefix and a moving last-block breakpoint are
therefore mutually exclusive unless we accumulate markers — which would exceed
Anthropic's four-breakpoint limit and still rewrite bytes on rotation.

**Decision:** the prefix invariant is defined over the *canonical request* —
the exact same request with `cache_control` decorations removed — and is
asserted over those bytes minus the trailing `]}` envelope, matching ticket #15.
Breakpoints are wire-only decorations applied at encode time; they are never
stored in canonical history and never replayed. Raw provider content blocks are
stored verbatim (thinking, signatures, tool_use, original key order) and replayed
byte-identically.

Consequences:

- OpenAI-compatible requests have no explicit breakpoints, so canonical bytes
  **are** the full wire bytes and the full wire golden prefix holds as well.
- Anthropic wire bytes legitimately differ across turns by breakpoint movement
  and appended messages; the canonical prefix never changes. A test asserts
  `strip(wire) == canonical` so the decorations cannot leak into stored history.
- Compaction remains the only allowed prefix reset (ADR-0001); no keepalive
  request is ever sent, so turn count equals request count.

Breakpoint rules implemented: 1h at the end of the static system block, 5m on
the last block, a third at the 20th-from-last block only when a turn adds more
than 20 blocks (Anthropic lookback limit), and nothing else.

System instruction is a constant with no date, cwd, git state or workspace
value; dynamic information is appended only as a tail context note on the new
user turn and becomes canonical history once sent.

## Evidence

- `go test ./... -count=1` — all packages pass.
- Seam 1: `tests/integration/layout_seam1_test.go` asserts canonical golden
  prefix for both adapters, hashed `prompt_cache_key`/`session_id`, breakpoints,
  raw block replay through SQLite, provider-specific usage fields and no
  keepalive requests.
- Adapter level: `internal/model/adapters/{openai,anthropic}/layout_test.go`
  assert exact wire bytes, breakpoint counts and usage field parsing.
