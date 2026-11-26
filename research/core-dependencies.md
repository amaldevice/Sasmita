# Core dependencies (status 25 Nov 2026)

## Koreksi terhadap TAD v0.2.0
- `mygo.egoist.dev/docs` redirect 308 ke `mygo.run/docs`.
- Sumber kanonik modernc sqlite: gitlab.com/cznic/sqlite.
- Docs Eino Chapter 7 menulis `Put/Get(*Checkpoint)`; source (`compose/checkpoint.go`) memakai `Get/Set([]byte)`. Ikuti source.
- TAD tidak menyebut MyGo butuh Go 1.27.3+ dan repo baru dibuat 2026-09-26.

## MyGo v0.10.0 (2026-11-17, MIT, ~6.9k stars, 177 open issues)
- Ada: OS WebView, generated TypeScript client, channels, typed events, native GPU UI, signing/notarization, tray, menu, dialog, multi-window (`docs/distribution.md`, `docs/windows.md`).
- `PageOptions.Untrusted` memblokir Go calls, permissions, broadcasts (`docs/frontend.md`).
- Tidak ditemukan API CSP atau sandboxed iframe.
- Verdict: conditional. Repo ~8 minggu, pre-1.0. Wajib spike.

## Wails v3 (v3.0.0 stable Okt 2026, v3.1.0 2026-11-01, MIT, ~37k stars)
- Multi-window, tray, menu, dialog, signing, updater terdokumentasi. Backend macOS tanpa CGO masih experimental.
- Custom protocol / isolasi belum diverifikasi.
- Verdict: fallback terkuat, uji di spike yang sama.

## Eino v0.9.23 (2026-11-09, eino-ext Apache-2.0)
- `adk.SendEvent`, `adk.WithCancel`, interrupt/resume ada (pkg.go.dev/adk). Resume lewat `CheckPointStore`, jadi store SQLite memungkinkan. Tipe checkpoint harus didaftarkan dengan `schema.RegisterName`.
- `TypedGenModelInput` dan `WrapModel` mengontrol message array.
- Model resmi: openai (base URL bisa diatur), claude, gemini, ollama, openrouter, deepseek. Claude punya `AutoCacheControl`.
- Risiko cache: komponen Claude menggabungkan tool result dan system message yang berdekatan, jadi urutan di wire bisa beda dari array aplikasi (`claude.go`).
- Verdict: pakai di belakang `AgentEngine`, setelah spike checkpoint.

## MCP go-sdk v1.8.0 (2026-09-14)
- Spec 2026-07-28 plus 2025-11-25 dan lebih lama. Client stdio dan Streamable HTTP ada. OAuth client experimental.
- Tidak ada dukungan MCP Apps (`ui://`).
- Lisensi campuran (kode baru Apache-2.0, lama MIT).
- Verdict: pakai. Tunda MCP Apps.

## modernc.org/sqlite v1.64.0 (2026-11-23)
- darwin/arm64 didukung. Benchmark vendor: kerja CPU-bound 1.3–2.0x waktu C SQLite (Linux amd64).
- FTS5 tampaknya ter-compile (docs menyebut shadow tables), belum dikonfirmasi langsung.
- v1.63.0 pindah ke `SQLITE_OPEN_NOMUTEX`.
- Verdict: pakai, benchmark FTS5/WAL di darwin/arm64 dulu.
