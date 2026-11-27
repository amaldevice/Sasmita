# Ticket #26 — hybrid tool discovery

## What is verified locally

Seam-1 (fake provider, exact wire bytes) verifies:

- `find_tools` ranks enabled tools from a local FTS index, returns full schemas
  in a tool result capped at 8 KB, and leaves the frozen tool list unchanged.
- `call_tool` resolves to the real tool before schema validation, policy and
  approval; the approval card and its stored record carry the real tool ID.
- `load_skill` returns bounded skill content as a tool result.
- Tools used through `call_tool` become native definitions only at compaction;
  the pre-compaction request list is byte-stable apart from the allowed prefix
  reset.
- The dispatcher enforces per-tool concurrency, a hard per-call timeout and
  egress labels (unit tests in `internal/tools/discovery`).

Promotion state is process-local: a restart rebuilds the frozen base list. A
durable promotion table was not added because no migration is essential for the
seam-1 contract and compaction already resets the prefix.

## S3 behaviour gate — not measured

The TAD/spike-plan S3 behaviour gate (≥90% tool-selection accuracy of discovery
versus all-native tools) needs a real, credentialed model and a labelled task
set. Neither is available in this environment; all seam-1 runs use scripted
providers. No accuracy number is claimed and no architecture change was made on
the basis of speculation. Run the gate with a real provider before treating
discovery accuracy as proven.
