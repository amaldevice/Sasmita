# Artifacts dan GenUI (status 25 Nov 2026)

## Claude
- Artifact berjalan di sandboxed iframe di origin terpisah claudeusercontent.com, diisolasi CSP + atribut sandbox. https://simonwillison.net/2025/jun/25/ai-powered-apps-with-claude/
- AI-powered artifact memanggil `window.claude.complete()`, ditagih ke akun viewer. https://claude.com/blog/build-artifacts
- "Custom visuals" inline (Maret 2026): HTML yang dibuat per jawaban, dirender di chat, ephemeral. https://support.claude.com/en/articles/13979539-custom-visuals-in-chat-and-cowork
- Daftar library yang diizinkan belum terverifikasi dari sumber resmi. ChatGPT/Gemini Canvas belum diriset.

## Open-source
- LibreChat: React preview via Sandpack, bundler dari CDN CodeSandbox default; self-host terdokumentasi.
- Open WebUI: iframe memakai `allow-scripts` + `allow-same-origin`, yang menurut advisory GHSA-xc8p-9rr6-97r2 menihilkan sandbox. Pelajaran: jangan pernah gabungkan keduanya.
- Biaya memori belum diukur.

## Protokol
- A2UI 1.0 stable 11 Nov 2026. Renderer: Lit, Angular, React, Flutter, SwiftUI/Compose. Tidak ada Svelte; `@a2ui/web_core` bisa jadi basis.
- MCP Apps: extension, bukan core spec. Spec 2026-01-26: `ui://` resource, CSP di `_meta.ui.csp`, postMessage JSON-RPC bridge.
- AG-UI: event-stream transport agent<->frontend, bukan format UI.

## WKWebView
- Site isolation menaruh site berbeda di WebContent process terpisah, jadi iframe cross-site bisa masing-masing jadi satu process.
- Custom scheme butuh `WKURLSchemeHandler`.
- Memori per iframe dan dukungan Worker belum terverifikasi.

## Token cost update
- Claude disebut beralih ke edit inline tertarget untuk perubahan kecil, full rewrite bila perlu (sumber sekunder).

## Rekomendasi
- Tier 1: declarative block, schema kecil, Svelte native, tanpa iframe.
- Tier 2: sandboxed artifact via `srcdoc`/custom scheme dengan opaque origin, `sandbox="allow-scripts"` saja, postMessage bridge, mount lazy, destroy saat off-screen.
- Edit: exact-match old/new replace yang gagal pada 0 atau >1 match, fallback full rewrite.
- Ukur memori iframe + WebContent process sebelum commit Tier 2.
