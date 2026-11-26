# Code Mode (status 26 Nov 2026)

Code Mode: model tidak memancarkan tool call satu per satu, tetapi menulis potongan kode (biasanya JS/TS, kadang Python) yang memanggil tools sebagai fungsi. Harness menjalankan kode itu di sandbox. Setiap pemanggilan tool di dalam kode ("inner call") dikirim balik ke host, dan hanya output akhir (`return`, `console.log`, `text()`) yang masuk ke context model. Istilah "Code Mode" dari Cloudflare; Anthropic menyebut varian API-nya "programmatic tool calling" (PTC). Gagasannya lebih tua: CodeAct (ICML 2024).

Metode: sumber primer (blog resmi, docs, source code yang di-clone pada 26 Nov 2026, GitHub API untuk issue/PR). Context7 dipakai untuk Cloudflare Agents dan smolagents. Benchmark lokal qjs/goja dijalankan di mesin ini (Apple A18 Pro, 8 GB, Go 1.27). Klaim yang hanya dari sumber sekunder ditandai.

## 1. Implementasi

### Cloudflare Code Mode (Agents SDK, `@cloudflare/codemode` 0.5.6)
- Asal: blog 26 Sep 2025. Tools MCP dikonversi jadi API TypeScript lengkap dengan doc comment, model diberi satu tool yang menjalankan TS, dan hasil kembali lewat `console.log`. Alasannya: model sudah melihat banyak kode TS nyata, sedangkan contoh tool call kebanyakan sintetis. Versi awal memuat seluruh API ke context. https://blog.cloudflare.com/code-mode/
- Sandbox: Dynamic Worker Loader (V8 isolate). Start dalam hitungan milidetik, beberapa MB, satu isolate baru per snippet. `fetch()`/`connect()` throw (`globalOutbound: null`). Akses keluar hanya lewat binding RPC ke MCP server, dan API key dipegang supervisor sehingga kode tidak bisa membocorkannya. Sumber sama.
- Server-side Code Mode (20 Feb 2026): MCP server Cloudflare API hanya punya `search()` dan `execute()`, masing-masing menerima kode JS. Lebih dari 2.500 endpoint muat dalam ~1.000 token, dibanding 1,17 juta token bila tiap endpoint jadi tool (pengurangan 99,9%, diukur dengan tiktoken). Spec OpenAPI tidak pernah masuk context dan hanya di-query lewat kode. Token OAuth di-downscope ke izin yang disetujui user. https://blog.cloudflare.com/code-mode-mcp/
- Executor: `DynamicWorkerExecutor` (timeout default 60 s, outbound null) dan `IframeSandboxExecutor` untuk browser (`sandbox="allow-scripts"`, CSP `default-src 'none'; script-src 'unsafe-inline' 'unsafe-eval'`, timeout 30 s, dispatch lewat postMessage). Output dipotong struktural, default 6.000 token. Source: `packages/codemode/src/executor.ts`, `iframe-executor.ts`, `truncate.ts` (github.com/cloudflare/agents @ d2f32ac).
- Discovery di dalam sandbox: `codemode.search(query)` dan `codemode.describe(target)` mengembalikan hasil ke kode yang sedang berjalan, bukan ke prompt. Search berbasis skor token nama/deskripsi, maksimal 50 hasil. https://github.com/cloudflare/agents/blob/main/docs/codemode/runtime.md
- Approval per inner call: tool bertanda `requiresApproval: true` membuat run abort dan dicatat sebagai `pending`. Setelah user approve, kode dijalankan ulang dari awal. Call yang sudah ada di log di-replay dari log, call yang di-approve dieksekusi sungguhan. Ada `reject` dan `rollback` (revert terbalik untuk tool yang punya `revert`). Syaratnya kode deterministik terhadap urutan tool call. Divergensi dideteksi dan dikembalikan sebagai `{status:"error"}`. Nilai nondeterministik harus dibungkus `codemode.step()`. Hindari `Promise.all` pada run yang mungkin pause karena urutan kedatangan call bisa berubah. https://github.com/cloudflare/agents/blob/main/docs/codemode/approvals.md
- Batasan: `createCodeTool` lama masih mengeluarkan tool `needsApproval` dari codemode, jadi approval hanya bekerja lewat connector baru. Kualitas kode tergantung model. Executor timeout harus ≥ timeout elicitation (60 s) karena waktu tunggu manusia ikut dihitung. https://github.com/cloudflare/agents/blob/main/docs/agents/codemode.md, `examples/codemode-mcp-openapi/README.md`

### Anthropic "Code execution with MCP" (blog, Nov 2025)
- Tools disajikan sebagai file tree (`./servers/<server>/<tool>.ts`). Agent melakukan `ls` lalu membaca hanya file tool yang dibutuhkan (progressive disclosure). Alternatifnya tool `search_tools` dengan parameter detail level (nama saja / plus deskripsi / schema penuh). Contoh ilustratif turun dari 150.000 ke 2.000 token (98,7%). Ini satu skenario, bukan benchmark. https://www.anthropic.com/engineering/code-execution-with-mcp
- Data antara tetap di environment eksekusi. Harness bisa men-tokenize PII sebelum sampai ke model lalu men-untokenize saat diteruskan ke tool lain. State dan skill bisa disimpan sebagai file. Anthropic sendiri mengingatkan biaya operasional sandbox: isolasi, resource limit, dan monitoring.

### Anthropic programmatic tool calling dan tool search (API)
- PTC: tool diberi `allowed_callers: ["code_execution_20260120"]`. Claude menulis Python di container code execution milik Anthropic. Setiap inner call muncul ke client sebagai `tool_use` dengan field `caller`, client mengeksekusi lalu membalas `tool_result`. Hasil inner call tidak masuk context dan tidak ditagih sebagai token. https://platform.claude.com/docs/en/agents-and-tools/tool-use/programmatic-tool-calling
- Bukti token dari docs: −38% billed input token pada benchmark 75 tool dengan akurasi sama. Pada τ²-bench (1–2 call sekuensial per turn) skor sama tetapi biaya **+8%**. Di trafik produksi dengan 10–49 tool, penghematan tipikal 20–40%. Blog: rata-rata 43.588 → 27.297 token (−37%) pada riset kompleks, knowledge retrieval 25,6→28,5%, GIA 46,5→51,2%. https://www.anthropic.com/engineering/advanced-tool-use
- Batasan: tidak ada untuk Haiku 4.5. Tidak kompatibel dengan `strict: true` dan `disable_parallel_tool_use`. Tool dari MCP connector tidak bisa dipanggil programatik. Balasan untuk call yang pending hanya boleh berisi `tool_result` berupa teks. Pending call timeout sekitar 4 menit (`TimeoutError` di kode), jadi approval manusia harus selesai dalam jendela itu. Tidak eligible ZDR dan data container disimpan hingga 30 hari. `allowed_callers` bukan batas keamanan. Docs PTC sama.
- Tool search: tool `defer_loading: true` tidak dimuat di awal. Di contoh Anthropic, ~77K turun ke ~8,7K token (−85%), dan akurasi MCP eval naik (Opus 4: 49→74%, Opus 4.5: 79,5→88,1%). Menurut Anthropic, fitur ini tidak merusak prompt cache karena tool deferred dikecualikan dari prompt awal. Blog advanced-tool-use.

### Codex (openai/codex @ 5148e3d)
- Code Mode ada sebagai tool `exec`: JavaScript dijalankan di **V8 isolate baru** per cell, tanpa Node/fs/network/console. Tools ada di global `tools`, dengan helper `text()`, `image()`, `store()`/`load()` lintas `exec`, `yield_control()`, `exit()`, dan pragma `// @exec: {"yield_time_ms":…, "max_output_tokens":…}` (default 10.000 token). Script yang lewat batas `yield_time_ms` mengembalikan cell ID. Model lalu memanggil tool `wait` untuk mengambil output baru atau menghentikan cell. Input berupa raw JS, bukan JSON. Source: `codex-rs/code-mode-protocol/src/description.rs`.
- Adopsi: feature flag `code_mode` masih `UnderDevelopment` (default false), tetapi katalog model bawaan (`codex-rs/models-manager/models.json`) menetapkan `tool_mode: "code_mode_only"` untuk semua model gpt-5.6/6.x, dan `gpt-5.5` tanpa mode. Artinya model Codex saat ini berjalan dalam mode code-only. Discovery di dalam kode: `ALL_TOOLS`, plus `tools.tool_search` (BM25) di balik flag `code_mode_tool_search` (PR #51209). Namespace bisa dikecualikan (`excluded_tool_namespaces`) atau dibuat direct-only (`direct_only_tool_namespaces`) (`codex-rs/features/src/feature_configs.rs`).
- Policy: inner call dikirim ke `tool_runtime.handle_tool_call_with_source(..., ToolCallSource::CodeMode{cell_id,…})`, yaitu jalur yang sama dengan tool call biasa. Approval dan sandbox `exec_command` tetap berlaku. `exec` tidak boleh memanggil dirinya sendiri. Source: `codex-rs/core/src/tools/code_mode/mod.rs`.
- Angka penghematan resmi OpenAI: tidak ditemukan.

### Claude Code
- Default: MCP tool search (tool deferred, hanya nama + server instructions yang dimuat di awal). https://code.claude.com/docs/en/mcp.md
- Code Mode eksperimental (`CLAUDE_CODE_EXPERIMENTAL_CODE_MODE=1`, v2.1.323+, hanya Opus/Sonnet/Haiku 5.5 atau Fable 5.2, tidak lewat gateway, tidak untuk ZDR/HIPAA). Tool `REPL` menjalankan JS. Contohnya `` $`rg …` ``, `edit(...)`, dan mengembalikan hanya bagian yang perlu dibaca. https://code.claude.com/docs/en/code-mode.md
- Policy: rule `deny`/`ask` dan hook `PreToolUse` berlaku per inner call dengan nama tool biasa. Dalam mode non-auto, user ditanya per call **saat script berjalan** (live pause). Jawaban "No" menghentikan kode, sedangkan "No + komentar" menolak satu call dan kode berlanjut. Waktu di prompt tidak dihitung dalam batas 10 menit. Dalam auto mode, classifier meninjau **kode, bukan tiap call**, dan tidak melihat nilai runtime, jadi rule tetap diperlukan untuk call kritis. Bisa juga `ask: ["REPL"]` untuk menyetujui kode sebelum jalan.
- Keamanan: sandbox Claude Code hanya membungkus shell command. JS dan call file/web/MCP berjalan **di luar sandbox**. Ada `PostToolUse` untuk me-redact output.

### opencode (anomalyco/opencode @ 90dc1b5)
- `@opencode/codemode` 2.4.1: interpreter murni untuk bahasa mirip JS (psychoscript). Tidak mencoba men-sandbox JS arbitrer, tetapi hanya menjalankan fitur yang diimplementasi. Ada hook `tool.before` yang bisa menolak call, limit waktu/jumlah call/output, serta **suspend/resume dengan snapshot JSON** dan tool `idempotent`/`durable`. https://github.com/anomalyco/opencode/tree/dev/packages/codemode
- Adopsi: dimulai sebagai flag eksperimental (PR #34677, #35185, Jul 2026): saat aktif, tool MCP top-level disembunyikan dan child call lewat jalur permission native. Saat ini server MCP di config bisa opt-out dengan `codemode: false` (issue #58476), yang berarti code mode dipakai secara default untuk MCP.
- Masalah yang tercatat:
  - Tidak ada timeout untuk `execute`/plugin call (#54220).
  - Built-in tools (`shell`, `read`) tidak ada di katalog (#60014).
  - `search()` yang didokumentasikan tidak terdaftar (#48912).
  - Client ACP hanya melihat blob kode `execute`, bukan tool dan argumennya (#58476).
  - Katalog code mode ditempel di deskripsi tool `execute`, sehingga server MCP yang masuk di tengah turn mengubah daftar tool dan provider Claude Agent di-restart (t3code PR #30267). Ini anti-pattern caching yang sama dengan ADR-0001.

### pi (earendil-works/pi, dulu badlogic/pi-mono)
- Pi 1.0 menambahkan Codemode + MCP (PR #10040). Paket `@earendil-works/pi-codemode` menjalankan JS di **QuickJS-WASM di worker thread** per eksekusi (~20 ms termasuk pembuatan VM), tanpa timer/fetch/fs/modules, dengan `memoryLimitBytes`, `store`/`load`, dan output maksimal 16 Mi karakter. Hasil besar ditulis ke file JSON sementara. Tool `codemode` dibuat kompatibel dengan `exec` Codex. Ada mode `start()` deterministik (seed, waktu tetap, urutan hasil) untuk durable replay. https://github.com/earendil-works/pi/blob/main/packages/codemode/README.md
- Ronacher: Codemode menjalankan orkestrasi di sisi harness ("brain"), bukan di execution environment. Default hanya aktif bila MCP aktif. Hook `beforeToolCall` hanya berlaku bila inner call dilewatkan lewat `runToolCall()`/`ctx.executeTool()`. Pi membatasi 4 eksekusi tool konkuren. Motivasi utamanya model seperti Jev yang bekerja lebih baik dengan sandbox komposisi. Kritiknya: "Codemode di dalam Codemode" (MCP server Cloudflare) memaksa double JSON escaping yang membingungkan model kecil, dan banyak MCP server belum mengembalikan `structuredContent`. https://lucumr.pocoo.org/2026/10/6/codemode/
- Batasan QuickJS: interpreter, lambat untuk komputasi berat. Di Cloudflare ia memblok thread sampai kode await tool, sehingga dibutuhkan interrupt budget (default 100.000 poll ≈ 2 s loop ketat di M-series).

### Lainnya
- CodeAct (Wang dkk., ICML 2024): aksi berupa Python executable, 17 LLM, hingga +20% absolut success rate dan hingga 30% lebih sedikit aksi. Peningkatan **lebih besar pada model open-source**, karena JSON tool call lemah di model open-source saat itu. https://arxiv.org/abs/2402.01030
- smolagents `CodeAgent`: tool sebagai fungsi Python, kode ditulis model di teks. `LocalPythonExecutor` **bukan** batas keamanan, jadi butuh E2B/Blaxel/Modal/Docker. `ToolCallingAgent` (JSON) tetap tersedia. Context7 `/huggingface/smolagents` (README, SECURITY.md).
- Goose mengimplementasi Code Mode (dirujuk blog Cloudflare Feb 2026; tautan blog Goose 404 saat dicek).
- Sekunder, belum diverifikasi: Factory melaporkan +3,5 poin Terminal-Bench dan −15% biaya model (dikutip di issue can1357/oh-my-pi #17941).

## 2. Perbandingan ringkas

| | Bahasa / sandbox | Discovery | Approval inner call | Bukti token |
|-|-|-|-|-|
| Cloudflare | TS/JS, V8 isolate (Worker Loader), iframe | `search`/`describe` di sandbox | abort-and-replay, durable | 99,9% (definisi API besar) |
| Anthropic PTC | Python, container Anthropic | tool search terpisah | client eksekusi tiap call, ~4 menit timeout | −20–40% tipikal, +8% di workload sekuensial |
| Codex | JS, V8 isolate | `ALL_TOOLS`, `tool_search` (flag) | jalur tool runtime biasa, `wait` untuk cell panjang | tidak dipublikasi |
| Claude Code | JS, in-process (di luar sandbox) | tool search | live pause per call, rule/hook | tidak dipublikasi |
| opencode | psychoscript interpreter | `search()` (sempat rusak) | hook `before`, snapshot suspend/resume | tidak dipublikasi |
| pi | JS, QuickJS-WASM di worker | tool search di Codemode | hook jika lewat `executeTool` | tidak dipublikasi |
| smolagents/CodeAct | Python, sandbox eksternal | semua tool di prompt | tidak ada built-in | +20% success, −30% aksi |

Pola yang sama di semua implementasi: data antara tetap di sandbox, dan yang masuk context hanya output yang di-log/return, dengan batas token (Codex 10K, Cloudflare 6K). State lintas eksekusi disimpan lewat `store`/`load` (Codex, pi) atau file (Anthropic).

Mode kegagalan yang tercatat:
1. Replay divergence dan `Promise.all` pada run yang di-pause (Cloudflare).
2. Promise yang tidak di-await dibuang diam-diam (Codex, pi).
3. Timeout tidak ada atau terlalu pendek untuk approval (opencode #54220, Cloudflare 60 s).
4. Katalog yang berubah di tengah sesi (t3code).
5. Fungsi discovery hilang atau mengembalikan list kosong tanpa error (opencode #48912, t3code #33542).
6. UI kehilangan visibilitas per tool (opencode #58476).
7. Classifier tidak melihat nilai runtime (Claude Code).
8. Double escaping pada server-side Code Mode (Ronacher).
9. Komputasi berat lambat di QuickJS.

Kompatibilitas model lemah: semua vendor membatasi fitur ini ke model tertentu (Claude Code hanya model 5.5, PTC tanpa Haiku 4.5, Codex lewat `tool_mode` per model). Cloudflare mencatat kualitas kode bergantung pada kapabilitas model. Codex dan pi menghindari kode di dalam string JSON (raw/freeform input, grammar Lark untuk constrained decoding). Provider OpenAI-compatible umumnya hanya punya function calling JSON, jadi kode harus di-escape di dalam argumen string, dan Ronacher menyebut pola ini sulit bagi model kecil. Bukti sebaliknya hanya CodeAct 2024 (Python di teks, model open-source lama). Untuk model lokal/murah, efeknya belum terbukti dan harus diukur.

## 3. Penilaian untuk Sasmita

### Sandbox mana
Hasil uji lokal (A18 Pro, 8 GB; `fastschema/qjs` v0.0.6 + wazero v1.9.0, goja master), skrip di /tmp, tidak di repo:

| Opsi | Cold start | RAM | Catatan |
|-|-|-|-|
| QuickJS-WASM via wazero (qjs) | 314 ms compile pertama; **64 ms** proses baru dengan disk compilation cache; 1,7 ms per runtime berikutnya | RSS proses ~19–23 MB (cache), ~89 MB peak setelah 4 runtime + goja | 20 async host call ≈ 0,5 ms. `MemoryLimit`, `MaxExecutionTime`, context cancel tersedia. ES2023 + async/await. |
| goja | <1 ms | berbagi heap Go, tanpa batas memori per VM | ES5.1 + sebagian besar ES6, async/await jalan. `Interrupt()` ada, tetapi tidak ada isolasi memori. |
| JS Worker di WKWebView tersembunyi | belum diukur | WebContent process terpisah, belum diukur | Bridge Go→WebView→Worker→Go panjang. Blok network bergantung CSP. Menaruh eksekusi di sisi UI bertentangan dengan TAD (UI tidak mengeksekusi). |
| Sandboxed iframe | belum diukur | sama | Pola `IframeSandboxExecutor` Cloudflare sudah ada sebagai referensi. Masalahnya sama dengan Worker. |
| Pyodide (Worker, WebView terpisah) | detik | 420–556 MB dengan pandas | Terlalu berat untuk orkestrasi tool. Tetap untuk analisis data. |

**Temuan keamanan:** qjs secara default me-mount `os.Getwd()` sebagai `/` dan mengekspos modul `qjs:std`. Uji `std.loadFile('/secret.txt')` berhasil membaca file dari CWD. Wajib memakai `CWD` ke direktori kosong sekali-pakai, lebih baik lagi build wasm tanpa `qjs:std`/`qjs:os`. Repo qjs terakhir di-push Des 2025 (risiko maintenance). Alternatifnya `aperturerobotics/go-quickjs-wasi-reactor` (QuickJS-NG upstream WASI reactor) atau mengambil `quickjs-wasi` yang dipakai pi.

Rekomendasi sandbox: **QuickJS-WASM in-process di Go via wazero**, satu VM per eksekusi, dengan `MemoryLimit` ~32 MiB, deadline wall-clock, dan goroutine terpisah. Alasannya: policy engine, run manager, dan SQLite sudah ada di Go (TAD §ADR-006, "UI tidak mengeksekusi"). Tidak ada proses atau WebView tambahan, dan perilakunya sama dengan pi (engine sama). Seatbelt tidak diperlukan untuk VM ini karena tidak punya syscall selain import yang kita daftarkan. Seatbelt tetap untuk shell tool yang dipanggil dari dalam script.

### Policy dan approval per inner call
- Setiap `tools.x(args)` di VM memanggil satu host function Go. Fungsi ini meneruskan call ke **jalur yang sama dengan tool call top-level**: validasi schema, `Policy.Evaluate(tool,args)`, approval, dan eksekusi. Metadata yang dibawa: `source=script`, `parent_tool_call_id`, dan `seq`, mengikuti pola Codex `ToolCallSource::CodeMode` dan pi `parentToolCallId`. Tidak ada jalan pintas.
- Pause di tengah script: host function mengembalikan promise yang belum di-resolve, lalu goroutine menunggu keputusan approval. QuickJS diam karena tidak ada timer. Deadline dihentikan selama menunggu approval (seperti Claude Code dan catatan timeout Cloudflare). Jika ditolak, promise di-reject dengan error yang bisa ditangkap script. Tombol "Stop" menghentikan VM.
- Restart atau crash saat pause: VM hidup tidak bisa dipulihkan. Tahap 1: tandai eksekusi `interrupted`, simpan log inner call di SQLite (dibutuhkan juga untuk audit), dan laporkan ke model sebagai tool result. Tahap 2 opsional: abort-and-replay ala Cloudflare dari log tersebut. Syaratnya await sekuensial dan nilai nondeterministik di-seed (pi `start()` memberi seed/waktu tetap).
- UI menampilkan setiap inner call sebagai item tersendiri (nama tool + argumen, diff), bukan blob kode (pelajaran opencode #58476). Kartu approval menampilkan argumen runtime yang sebenarnya.
- Risiko komposisi: script bisa membaca data sensitif lalu meneruskannya ke tool jaringan dalam satu eksekusi. Policy per call melihat argumen, tetapi tidak melihat asal data. Untuk v1 cukup: tool dengan efek eksternal (network, write, send) selalu `ask` bila dipanggil dari script, kecuali user memberi izin eksplisit.

### Efek ke prompt caching (ADR-0001)
- Positif: `run_script(code)` adalah satu tool tetap yang bisa dibekukan per conversation. Hasil inner call tidak pernah masuk history, sehingga history tumbuh lebih lambat dan cache write lebih kecil. Output script masuk sebagai tool result biasa (append-only).
- Wajib: katalog tool **tidak boleh** ditaruh di deskripsi `run_script` (opencode/t3code membuktikan ini mengubah daftar tool di tengah turn). Discovery harus lewat `find_tools` (hasilnya di-append) atau `tools.find(...)` di dalam VM (hasil tidak masuk context sama sekali). Deklarasi TS yang di-render harus deterministik byte-per-byte.
- `store`/`load` lintas eksekusi disimpan di state run (SQLite), bukan di system prompt.
- Anthropic PTC native tidak cocok sebagai jalur utama: hanya Claude, data inner call melewati container Anthropic (tidak ZDR, disimpan 30 hari, bertentangan dengan local-first), dan timeout ~4 menit untuk approval.

## 4. Rekomendasi: **complement, ditahapkan** (bukan replace)

1. **Sekarang:** lanjutkan desain hybrid (core tools native + `find_tools` + `call_tool`). Rancang dispatcher `call_tool` sebagai satu fungsi Go `Dispatch(ctx, source, tool, args)` yang nanti dipakai juga oleh host function VM. Pola ini sudah konvergen di Codex, Cloudflare, dan pi: discovery via search, lalu eksekusi via satu entry point.
2. **Setelah hybrid stabil:** tambahkan `run_script` sebagai core tool tetap ketiga, diaktifkan per model lewat capability flag (seperti `tool_mode` Codex), default **off** untuk model lemah atau tidak dikenal. Model menulis JS di dalam argumen string. Evaluasi dengan golden task: fan-out (≥3 call dependen atau puluhan item), hasil besar, dan task sekuensial sebagai kontrol (PTC naik +8% biaya di sini).
3. **Jangan replace:** bukti penghematan besar berasal dari (a) definisi tool yang sangat banyak, yang sudah ditangani `find_tools`, dan (b) fan-out/filter hasil besar. Untuk alur sekuensial, Code Mode tidak menghemat. Model OpenAI-compatible yang lemah juga belum terbukti nyaman menulis kode di argumen JSON.
4. **Defer** durable replay, rollback, dan snapshot sampai ada kebutuhan nyata. Live pause + status `interrupted` saat restart sudah cukup untuk v1.

Belum diverifikasi: memori per WKWebView/Worker, perilaku qjs build tanpa `qjs:std`, akurasi model OpenAI-compatible (Qwen/DeepSeek/lokal) saat menulis kode di argumen tool, dan angka penghematan internal Codex/Claude Code/pi/opencode (tidak dipublikasi).
