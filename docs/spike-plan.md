# Spike plan Phase 0

Hasil dari ticket #10 (map #1). Lampiran untuk TAD v0.3.

## Aturan

- Setiap spike adalah kode throwaway di branch `spike/<nama>`. Hasil angka ditulis ke `docs/benchmarks/<nama>.md`.
- Timebox 3 hari kerja per spike. S1 dan S2 dijalankan paralel bila ada agent yang mengerjakan.
- Mesin ukur: Mac A18 Pro, RAM 8 GB, macOS 27.x. Ukur build release yang dipaketkan, agregat seluruh process tree.

## Model target (S2, S3)

- OpenAI-compatible langsung: DeepSeek.
- OpenAI-compatible lewat gateway: satu model di OpenRouter (dipilih saat spike).
- Anthropic: Claude (pembanding).
- Anthropic-compatible pihak ketiga: satu endpoint (dipilih saat spike). Adapter Anthropic wajib menerima base URL yang bisa diatur.
- Lokal: satu model lewat llama.cpp atau Ollama.

## Spike

Urutan sesuai risiko. S1 dan S2 lebih dulu.

### S1 MyGo shell + isolasi
Sumber: TAD Phase 0 #1, ticket #7, #8, #9, checklist `research/mygo-isolation.md` (branch `research/mygo-isolation`).
- App MyGo + Svelte dipaketkan: startup, RSS idle/chat, IPC request dan streaming channel.
- Checklist isolasi iframe 1–11 (bridge, CSP header, srcdoc, Origin/CallerPage, navigasi, Worker, proses dan memori per iframe, `while(true)`, window `Untrusted`, latensi postMessage, ContentRules).
- Watchdog: deteksi main thread beku dan reload.
- Window `Untrusted` tersembunyi menjalankan module worker Pyodide: RAM, cold start, isolasi crash/OOM.
- COOP/COEP lewat custom scheme (SharedArrayBuffer). CSP `connect-src 'none'` memblokir `js.fetch` dari worker.
- three.js WebGL di iframe sandbox, memori GPU, rAF saat off-screen.
- Capture snapshot iframe opaque origin.
- Buka ulang conversation panjang dengan banyak artifact.
- Waktu transpile esbuild untuk JSX 50–500 KB. Ukuran artifact runtime di installer.

### S2 Adapter model + cache
Sumber: TAD Phase 0 #2 dan #5, ticket #2, #3.
- Adapter sendiri Anthropic dan OpenAI-compatible yang mengimplementasikan `ToolCallingChatModel`.
- Golden test: request turn N terserialisasi = prefix byte-exact request turn N+1.
- Hit rate nyata di sesi 30+ turn untuk setiap model target.
- `reasoning_content` model reasoning: wajib dikirim balik atau ditolak, dan dampaknya ke byte-stability.
- Anthropic: urutan TTL 1h sebelum 5m.
- llama.cpp/Ollama: slot/KV cache saat dua conversation bergantian, trade-off `keep_alive` vs RAM.
- Eino ADK: streaming, cancel, interrupt/resume dengan checkpoint di SQLite setelah restart.

### S3 Perilaku model (setelah S2)
Sumber: ticket #6, #8.
- Akurasi `find_tools` + `call_tool` vs tool native pada 20 tugas fixture.
- Kepatuhan visual-first (create_artifact/render_ui dulu, teks sesudahnya) pada 20 prompt penjelasan, plus token dan latensi tambahan.
- Token Code Mode vs native parallel tool calls pada tugas fan-out (web search) dan sekuensial.

### S4 Runtime eksekusi
Sumber: TAD Phase 0 #6, ticket #6, #9.
- QuickJS-WASM via wazero: host function async (Promise.all benar-benar paralel), FS kosong, status maintenance `fastschema/qjs`.
- `query_table`: muat CSV/XLSX 50 MB ke SQLite, waktu dan memori.
- Profil Seatbelt turunan Codex: `npm test`, `go test`, `git status` jalan; network dan tulis di luar workspace ditolak.
- Path scoping dan symlink escape untuk tool file.

### S5 Persistence + MCP
Sumber: TAD Phase 0 #3 dan #4.
- modernc.org/sqlite: FTS5, WAL, migrasi, backup/restore, tulis event konkuren.
- MCP go-sdk: stdio dan Streamable HTTP, disconnect, timeout, schema rusak, hasil besar.

## Gate

| Area | Lulus bila |
|---|---|
| Resource | RSS agregat idle ≤150 MB, chat ≤250 MB, startup ≤2 s. 2 iframe hidup + 1 WebGL ≤ +150 MB di atas baseline chat |
| Isolasi (mutlak) | Pesan iframe tidak pernah sampai ke Go, CSP dihormati, navigasi keluar iframe gagal |
| Watchdog | Freeze terdeteksi dan UI pulih ≤5 s |
| Pyodide | Cold start ≤3 s di window tersembunyi, OOM worker tidak mengganggu window utama |
| Cache | Golden test prefix 100%. ≥80% input token cache hit mulai turn ke-3 di DeepSeek dan OpenRouter |
| Perilaku model | `find_tools`/`call_tool` ≥90% dan ≤5 poin di bawah native. Visual-first ≥90% |
| Seatbelt | Semua kasus allow/deny lulus |

## Bila gate gagal

| Kegagalan | Tindakan |
|---|---|
| Pesan iframe bocor ke Go | Tahan Tier 2 inline; artifact hanya di window `Untrusted` sampai MyGo di-patch (sumbang ke upstream) |
| Watchdog tidak mendeteksi freeze | Artifact hanya di window terpisah |
| Window `Untrusted` tersembunyi tidak bisa menjalankan Pyodide | Analyst hanya `query_table` sampai ada solusi |
| Cache hit <80% | Perbaiki adapter; bukan alasan mengubah arsitektur |
| Perilaku model di bawah gate | Model ditandai "dukungan terbatas", fitur terkait dimatikan untuk model itu |
| MyGo gagal mendasar (IPC, packaging, memori jauh di atas target) | Evaluasi ulang Wails v3 |

## Urutan phase setelah spike

| Phase | Isi |
|---|---|
| 1 | Chat inti, adapter Anthropic(-compatible) + OpenAI-compatible, persistence, metrik cache di tampilan usage |
| 2 | Agent runtime: run manager, Eino ADK, context note + compaction, `find_tools`/`call_tool`, tool aman bawaan |
| 3 | GenUI: UI block + sandboxed artifact, visual-first, watchdog |
| 4 | Analyst (`query_table` + Pyodide), MCP, harness profile, approval, Keychain |
| 5 | Long-form, harness Coding (Seatbelt shell) |
| 6 | Code Mode `run_script`, MCP Apps, AI-powered artifact, vector retrieval, multi-agent |
