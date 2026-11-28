# Sasmita

Sasmita is a local-first AI workspace for macOS. Ask a question and get an interactive visual with the explanation underneath. Conversations, artifacts, and settings stay on your Mac; you choose the model provider.

The current build is an early preview published on [GitHub Releases](https://github.com/amaldevice/Sasmita/releases/latest).

## Features

- **Visual-first answers.** Interactive UI blocks and sandboxed artifacts appear before the written explanation.
- **Your provider, your data.** OpenAI-compatible, Anthropic-compatible, or local endpoints; conversation history and artifacts stay on your Mac.
- **Agent with approvals.** Bounded built-in tools, an expandable activity timeline, and approvals before risky actions.
- **Long-session workflow.** Conversation search, projects, compaction, restart recovery, and visible token/cache/cost figures.
- **Bilingual interface.** Full Indonesian and English copy, dark and light themes, and reduced-motion support.

The complete current inventory — tools, Modes, artifact lifecycle, and verification status — is in [FEATURE.md](FEATURE.md).

## Requirements

- macOS 12 or later on Apple Silicon
- An OpenAI-compatible or Anthropic-compatible endpoint, or a local model server
- No Sasmita account is required

## Installation

1. Download the `.dmg` from [GitHub Releases](https://github.com/amaldevice/Sasmita/releases/latest).
2. Open it and move `Sasmita.app` to **Applications**.
3. Right-click `Sasmita.app`, choose **Open**, then **Open** again on first launch. The app is ad-hoc signed, so macOS asks once before running it.
4. If macOS still blocks it: `xattr -dr com.apple.quarantine /Applications/Sasmita.app`
5. Optional integrity check: `shasum -a 256 ~/Downloads/*.dmg`, compared with `SHA256SUMS.txt` on the release page.

Installation is complete when the welcome screen appears. Updates are manual: download a newer DMG when a new version is published.

## Quick start

1. On the welcome screen, select a detected local server or choose **Use an API**.
2. Enter the base URL and API key, then run **Test connection**. The key is stored in the macOS Keychain and is never shown again.
3. Pick a Mode and model in the composer, attach a file if needed, and send your first message.
4. Ask how something works to see the visual-first answer: an artifact or UI block appears before the explanation.

<details>
<summary>Build from source</summary>

Requires macOS 12 or later (Apple Silicon), Go 1.27.3, and Node.js 22+. Versions are pinned in `go.mod` and `shell/mygo/go.mod`.

```sh
git clone https://github.com/amaldevice/Sasmita.git
cd Sasmita
./scripts/build-app.sh
```

The script installs the pinned MyGo CLI into a temporary directory, builds the frontend, embeds it, and checks the resulting `AppIcon.icns`, `Info.plist`, and DMG checksum. The build is complete when it prints `OK: build/darwin-arm64/ ...`. The tested build record is in `docs/benchmarks/package-build-final.md`.

A plain `go build` in `shell/mygo` is not a release build: that binary has no embedded frontend and shows a "no frontend" page. Use the script or download a DMG instead.

</details>

## Privacy and data

- SQLite database: `~/Library/Application Support/Sasmita/sasmita.db`, with artifacts stored alongside it. Set `SASMITA_DB_PATH` to move it.
- API keys live in the macOS Keychain; only secret references are stored in SQLite.
- Requests sent to a remote provider are marked in the interface. Risky tool calls require approval before execution.
- Sandboxed artifacts run in an isolated frame with network access disabled by policy.

## Scope

Sasmita v1 covers core chat, the agent runtime, and visual answers. MCP, the Data Analyst Mode with spreadsheet and PDF parsing, long-form generation, Windows and Linux builds, auto-update, and Developer ID notarization are planned for later releases. Packaged-app checks for WKWebView isolation and memory targets are recorded in `docs/benchmarks/`.

## Project documents

[Features](FEATURE.md) · [Product](PRODUCT.md) · [Design](DESIGN.md) · [Technical design](SASMITA_TAD_v0.3.0.md) · [Decisions](docs/adr/) · [Contributor conventions](docs/agents/)

## License

MIT — see [LICENSE](LICENSE). Third-party dependencies and their licenses are listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
