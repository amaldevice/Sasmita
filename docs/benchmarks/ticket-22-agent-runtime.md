# Ticket #22 — agent runtime notes

## Eino ADK boundary

`github.com/cloudwego/eino v0.9.23` is available locally and is used for real:
`internal/agent` implements `adk.Agent`, executes through `adk.NewRunner`, opts
into `adk.WithCancel()` (mapped to the #21 run manager's cancellation context),
and accepts an `adk.CheckPointStore` with the `Get/Set([]byte)` shape.

The ADK built-in `ChatModelAgent`/ReAct loop was **not** used. ADR-0002 requires
the application to own provider message serialization for the byte-stable
prefix; letting the ADK chat-model component build requests would invalidate
the #17 golden prefix guarantee. The custom ADK agent therefore drives the
app-owned `model.ModelProvider` port and tool dispatcher, and emits only domain
types across `agent.Engine`. No Eino type leaks into `appsvc`, bindings or UI.

Checkpoint storage is a wired seam, not an active approval flow: this ticket's
tools are safe and execute immediately, so interrupts/checkpoints arrive with
the approval policy (#23). `tool.proposed` carries `executed: false` for that
engine to consume; `tool.executed` / `tool.artifact` carry `executed: true`.

## Tool limits

| Tool | Risk | Timeout | Output cap |
|---|---|---|---|
| `calculator` | compute | 2 s | service cap (default 4096 B) |
| `read_project_file` | read_only | 2 s | service cap |
| `search` | read_only | 2 s | service cap |
| `write_artifact` | write | 2 s | 512 B result |

Files are read only from explicitly configured attached files or project roots;
symlinks and `..` are resolved before containment is checked. Output above the
cap is written as an opaque `artifact://<id>` file and only a short summary plus
that stable reference enters canonical history and the chat timeline.

## Known limits (honest deviations)

- Tool-call wire round-tripping was exercised against the Anthropic-compatible
  adapter, which already emits/normalizes `tool_use`. The OpenAI-compatible
  adapter still sends no tool schemas and parses no `tool_calls`; adding that is
  outside this ticket's lane.
- The host does not yet supply project roots or attachments to the registry, so
  file/search tools report "no files available" until a later ticket wires the
  project workspace. Schema, risk, timeout and caps are registered regardless.
- The S2 live cache-hit gate (≥80% from turn 3) was **not measured** — this
  machine has no provider credentials. The byte-stability and adapter work from
  #17 is unchanged; see `docs/benchmarks/s2-adapters-cache.md`.
- Meta-tools (`find_tools`/`call_tool`) and the approval policy are deliberately
  left to #26 and #23.
