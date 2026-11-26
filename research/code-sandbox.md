# Sandbox eksekusi kode di macOS (status 26 Nov 2026)

Menjawab issue #5. Sumber utama: docs resmi, repo GitHub, release API. Context7 dipakai untuk docs Pyodide dan wazero. Pengukuran lokal dilakukan di mesin dev: macOS 27.2 (26B123), Apple A18 Pro, RAM 8 GB, Node 24.16. Klaim yang belum diverifikasi ditandai **(unverified)**.

## TL;DR

- **Analyst (default, non-IT): Pyodide di module Web Worker.** Satu-satunya opsi dengan build resmi numpy + pandas. Tanpa install tambahan, tanpa VM, tanpa akses FS host. File masuk/keluar lewat `pyodide.FS` sebagai bytes. Jalankan di WebView terpisah (headless, CSP `connect-src 'none'`), bukan di WebView UI utama.
- **Coding (opt-in): shell host di dalam Seatbelt (`/usr/bin/sandbox-exec`) + approval per perintah.** Pola yang sama dipakai Codex CLI dan Claude Code. Masih berfungsi di macOS 27.2 walau berstatus DEPRECATED. Tulis hanya ke workspace, network mati default, `.git` read-only.
- **VM/container (Apple `container`, Docker, Colima, libkrun) bukan default.** Minimal ~1–2 GiB per VM terlalu mahal di mesin 8 GB. Simpan sebagai mode "isolasi kuat" opsional di macOS 26+.
- **CPython WASI via wazero belum layak untuk pandas.** Runtime-nya bagus (pure Go, preopen dir, tanpa socket), tapi tidak ada build numpy/pandas WASI yang resmi.

## Perbandingan

| | Pyodide (Worker) | CPython WASI + wazero | Seatbelt + Python host | Apple `container` | Docker / Podman / Colima | Micro-VM (libkrun, vz) |
|---|---|---|---|---|---|---|
| Batas keamanan | Sandbox WebContent + wasm. Python bisa akses JS global di worker (`js.fetch`) | Wasm, tanpa ambient authority. Hanya mount + host function yang diberi | Kebijakan MAC kernel pada proses host. Bukan VM | VM per container (Virtualization.framework) | Satu VM Linux bersama + namespace container | VM per sandbox (HVF) |
| RAM | Diukur: RSS Node 420–556 MB dengan pandas dimuat. Heap wasm maks 4 GiB | Unverified. `WithMemoryLimitPages` tersedia | ~0 overhead. Hanya proses Python itu sendiri | Default 1 GiB per container | Docker: 50% RAM host (4 GB di 8 GB). Colima: 2 GiB | Ditentukan sendiri, unverified |
| Cold start | Diukur (Node, cache warm): core 1.1 s, load numpy+pandas 0.4 s, import+run pertama 1.7–2.3 s | Unverified. Compile cache ke disk | Spawn proses (ms) | Klaim "sub-second", tanpa benchmark primer | VM boot detik; resource saver restart 3–10 s | Unverified |
| Library | numpy 2.4.6, pandas 3.0.2, scipy 1.18.0, python-calamine 0.6.2 built-in. openpyxl 3.1.5 via micropip (pure Python) | Tidak ada build resmi. Satu proyek pihak ketiga v0.1.0 (unverified) | Semua wheel CPython native | Semua (Linux arm64) | Semua | Semua |
| Ukuran paket | Core ~12.1 MB (wasm 9.6 MB + stdlib 2.5 MB). numpy 3.0 MB + pandas 4.2 MB + deps ~0.75 MB. Distribusi penuh 337 MB bz2 (tidak perlu) | ~250 MB unpacked (proyek pihak ketiga) | python-build-standalone 3.14 arm64 stripped 27 MB tar.gz + wheel | CLI + kernel + image (`python:3.13-slim` ~42.5 MB compressed) | App besar, install oleh user | Kernel + rootfs sendiri |
| Kontrol network | Lewat browser. Blok dengan CSP `connect-src 'none'` atau hapus akses `js` | Tidak ada socket di WASI 0.1 | `(deny network*)`, diuji memblok curl dan socket Python | IP per container; egress allowlist unverified | Networking Docker/gvproxy | virtio-net / TSI / tanpa net |
| File in/out | `FS.writeFile` / `FS.readFile` bytes, `unpackArchive`. Tidak ada mount native di WebKit | Preopen dir RO/RW (`WithDirMount`, `WithReadOnlyDirMount`) | Path allow/deny pada FS host | Bind mount | Bind mount (VirtioFS) | virtio-fs / shared dir |
| Syarat host | WKWebView, tidak ada install | Tidak ada (pure Go). Compiler mode butuh entitlement JIT di app signed | macOS apa pun yang masih punya `sandbox-exec` | macOS 26+, Apple silicon | Install terpisah, lisensi Docker Desktop | macOS 12+/14+, entitlement virtualization |

## 1. Pyodide di Web Worker

- Versi stabil 314.0.8 (13 Nov 2026), CPython 3.14.2, Emscripten 5.0.3. https://api.github.com/repos/pyodide/pyodide/releases/latest, https://cdn.jsdelivr.net/pyodide/v314.0.8/full/pyodide-lock.json
- Paket built-in: numpy 2.4.6, pandas 3.0.2 (deps python-dateutil, pytz, six), scipy 1.18.0, python-calamine 0.6.2, xlrd, pyarrow 22.0.0. openpyxl tidak built-in; `openpyxl-3.1.5-py2.py3-none-any.whl` (251 kB) pure Python sehingga bisa via `micropip.install` atau dibundel sebagai wheel lokal. https://pyodide.org/en/stable/usage/packages-in-pyodide.html, https://pypi.org/project/openpyxl/#files
- Ukuran wheel (diunduh dari CDN v314.0.8): numpy 2,960,568 B; pandas 4,177,749 B; python-dateutil 229,892 B; pytz 510,489 B; six 11,050 B; python-calamine 489,423 B; micropip 115,486 B. Core: `pyodide.asm.wasm` 9,597,892 B, `python_stdlib.zip` 2,545,550 B. Total bundle Analyst ~20 MB sebelum kompresi.
- Pengukuran lokal (Node 24, `loadPyodide` + `loadPackage(["numpy","pandas"])` + `read_csv`/`to_csv` lewat `pyodide.FS`): core 1,075–1,099 ms; paket 409 ms saat cache warm (6.5 s saat pertama kali unduh dari CDN); run pertama termasuk import pandas 1.7–2.3 s; RSS 420–556 MB. Angka di WKWebView belum diukur.
- Worker: harus module worker sejak 314.0.0. Interrupt butuh `SharedArrayBuffer` + `setInterruptBuffer` (tulis `2` = `KeyboardInterrupt`), yang butuh cross-origin isolation (COOP/COEP). Fallback timeout keras: `worker.terminate()` lalu cold start ulang. https://pyodide.org/en/stable/usage/keyboard-interrupts.html, https://pyodide.org/en/stable/project/changelog.html
- Tidak ada thread, multiprocessing, atau `subprocess`. https://pyodide.org/en/stable/usage/faq.html
- Batas memori: wasm32 maks 4 GiB. Opsi `maxMemoryBytes` (PR #6483) baru ada di main, belum rilis. https://github.com/pyodide/pyodide/blob/main/docs/project/changelog.md
- JSPI ada di Safari 27+ (diperlukan hanya untuk `run_sync`/I/O sinkron, tidak untuk pandas). https://caniuse.com/wf-wasm-jspi
- FS: `pyodide.FS` = Emscripten FS (MEMFS). `mountNodeFS` hanya Node; `mountNativeFS` butuh File System Access API yang tidak ada di Safari. Jadi di WKWebView file selalu disalin sebagai bytes. https://pyodide.org/en/stable/usage/api/js-api.html, https://pyodide.org/en/stable/usage/file-system.html
- Network: Python bisa memanggil `js.fetch`. Blokir dengan CSP worker (`connect-src 'none'`) atau isolasi worker dari UI. Perilaku CSP worker di WKWebView unverified. https://pyodide.org/en/stable/usage/wasm-constraints.html
- Loading: `file://` diblok same-origin. Sajikan aset lewat custom URL scheme (`WKURLSchemeHandler`) dari Go, yang juga bisa memberi header COOP/COEP (unverified untuk WKWebView).

**Varian headless.** Pyodide bisa jalan di Node 18+ atau Deno (permission model `--allow-read`/`--deny-net`). Ini memberi batas OS-level tambahan, tapi menambah runtime 40–100+ MB ke bundle. Pyodide di Deno belum diverifikasi. https://docs.deno.com/runtime/fundamentals/security

**Catatan WKWebView.** Proses WebContent terpisah dari app; kehabisan memori hanya membuat view blank. Batas memori WKWebView tidak bisa dinaikkan (jawaban forum Apple lama, unverified untuk macOS 26/27). https://developer.apple.com/forums/thread/21956, https://developer.apple.com/forums/thread/133449. Karena itu Python sebaiknya di WebView tersembunyi terpisah, supaya crash/OOM tidak menghapus UI chat.

## 2. CPython WASI via wazero

- CPython WASI Tier 2 sejak 3.13 (wasip1). Target main bergeser ke `wasm32-unknown-wasip3` (component model) yang tidak didukung wazero. https://peps.python.org/pep-0011/, https://peps.python.org/pep-0816/
- Build preview1 tidak resmi: `brettcannon/cpython-wasi-build` (3.14.10, 3.15.2). https://github.com/brettcannon/cpython-wasi-build/releases
- numpy/pandas: tidak ada build resmi (numpy #25859 masih terbuka). `dicej/wasi-wheels` tidak dirawat. `bkmashiro/wasi-wheels` numpy 1.26 beta, tanpa pandas, untuk componentize-py. `ClayGendron/wasi-python` v0.1.0 mengklaim numpy+pandas+scipy di wasip1 (~250 MB, dibuat Oktober 2026, wazero "runs with a switch" belum diuji penulis). https://github.com/numpy/numpy/issues/25859, https://github.com/dicej/wasi-wheels, https://github.com/bkmashiro/wasi-wheels, https://github.com/ClayGendron/wasi-python
- wazero v1.12.0 (29 Mei 2026): pure Go, tanpa CGO, compiler mode teruji di darwin/arm64, exception handling baru ditambahkan. `NewCompilationCacheWithDir`, `WithMemoryLimitPages`, `WithCloseOnContextDone`, `FSConfig.WithDirMount`/`WithReadOnlyDirMount`. https://github.com/wazero/wazero/releases/tag/v1.12.0, https://pkg.go.dev/github.com/tetratelabs/wazero
- Compiler mode di app signed + hardened runtime butuh entitlement `com.apple.security.cs.allow-jit` (dilaporkan di https://github.com/wazero/wazero/issues/2393).
- Tidak ada socket (WASI 0.1) dan tidak ada thread. Batas keamanan paling bersih di antara semua opsi, tapi payload Python adalah masalahnya.
- wasmtime-go butuh CGO dan tidak ada prebuilt untuk macOS arm64. https://github.com/bytecodealliance/wasmtime-go

Kesimpulan: pantau. Layak dipertimbangkan ulang bila numpy/pandas punya build wasip1 yang dirawat.

## 3. Seatbelt / sandbox-exec

- Man page `sandbox-exec(1)` dan `sandbox_init(3)` bertanda DEPRECATED, mengarahkan ke App Sandbox. `sandbox_init(3)` menyebut named profile (`kSBXProfileNoNetwork` dsb.) tidak boleh dipakai saat build dengan SDK macOS 27.0+. https://keith.github.io/xcode-man-pages/sandbox-exec.1.html, https://keith.github.io/xcode-man-pages/sandbox_init.3.html
- **Diuji lokal di macOS 27.2:** `/usr/bin/sandbox-exec -f p.sb` dengan `(deny network*)` dan `(deny file-write* (subpath "/Users"))` berhasil: curl gagal resolve, `socket.create_connection` Python dapat `EPERM`, `touch ~/x` ditolak. Tidak ada warning deprecation yang tercetak.
- Masih dipakai produksi:
  - Codex CLI: `codex-rs/sandboxing/src/seatbelt.rs` selalu memanggil `/usr/bin/sandbox-exec` (bukan dari PATH), dengan `seatbelt_base_policy.sbpl` (`(deny default)`, diturunkan dari kebijakan Chrome), `seatbelt_network_policy.sbpl`, dan policy dinamis untuk proxy/unix socket. https://github.com/openai/codex/tree/main/codex-rs/sandboxing/src
  - Anthropic `sandbox-runtime` (`srt`, "Beta Research Preview"): membangun profil SBPL saat runtime lalu `sandbox-exec -p`; bubblewrap di Linux; network lewat proxy allowlist domain. https://github.com/anthropics/sandbox-runtime, `src/sandbox/macos-sandbox-utils.ts`
- Batasan: profil inline (`-p`) besar bisa melebihi ARG_MAX (claude-code #73468, #73437), pakai `-f`. Proxy domain bisa ditembus domain fronting dan tidak inspeksi TLS. Seatbelt membatasi akses per proses tapi semua proses tetap berbagi kernel dan user yang sama. https://github.com/anthropics/claude-code/issues/73468
- Risiko jangka panjang: Apple bisa menghapus `sandbox-exec` kapan saja. Tidak ada pengganti publik untuk sandbox ad-hoc per perintah; App Sandbox berlaku untuk seluruh app, bukan child process tertentu.
- Python host: python-build-standalone 20261124, `cpython-3.14.10+20261124-aarch64-apple-darwin-install_only_stripped.tar.gz` 26,957,069 B. Ditambah wheel numpy/pandas/openpyxl native (~30–60 MB unpacked, unverified). https://github.com/astral-sh/python-build-standalone/releases/latest

## 4. Apple Containerization / `container`

- Butuh Apple silicon + macOS 26; versi lama tidak didukung. https://github.com/apple/containerization, https://github.com/apple/container
- `container` 1.7.1 (Nov 2026), bundel Containerization 0.54.0 (0.x, API Swift belum stabil). VM ringan per container, kernel default Kata 6.18. Klaim "sub-second start", tanpa benchmark primer.
- Default 1 GiB RAM + 4 CPU per container; builder VM 2 GiB. https://github.com/apple/container/blob/main/docs/resource-usage.md
- Dari Go: shell out ke CLI (API XPC tidak dijamin). Tidak ada Go SDK.
- Beberapa advisory path traversal / symlink escape di 1.2–1.5. https://github.com/apple/container/releases
- Tidak terpasang di mesin dev ini, belum diukur.

## 5. Docker Desktop / Podman / Colima

- Docker Desktop: VM default 50% RAM host, swap 1 GB; resource saver restart 3–10 s. Gratis hanya untuk <250 karyawan dan <$10M revenue. https://docs.docker.com/desktop/settings-and-maintenance/settings/, https://docs.docker.com/subscription-billing/desktop-license/
- Podman machine: Fedora CoreOS VM; Podman Desktop 1.21 mengganti provider default macOS ke libkrun. https://podman-desktop.io/blog/podman-desktop-release-1.21
- Colima: default 2 CPU, 2 GiB, 100 GiB disk, MIT, di atas Lima. https://github.com/abiosoft/colima
- Semua menuntut user non-IT menginstal dan menjalankan VM terpisah. Tidak cocok sebagai default.

## 6. Micro-VM

- libkrun (HVF di macOS): virtio-fs, virtio-net/TSI, vsock. Catatan resminya: batas VM tidak membatasi resource host yang diekspos lewat device. krunkit butuh macOS 14+. https://github.com/containers/libkrun, https://github.com/containers/krunkit
- Code-Hex/vz v4: Virtualization.framework dari Go, macOS 12+, CGO, binary harus codesigned dengan `com.apple.security.virtualization`. Kernel + rootfs dikelola sendiri. https://github.com/Code-Hex/vz
- microsandbox (beta, Go SDK, allowlist network). https://github.com/superradcompany/microsandbox
- Firecracker hanya Linux/KVM. https://github.com/firecracker-microvm/firecracker
- RAM dan boot time untuk opsi ini belum terverifikasi dari sumber primer.

## Pola approval shell di harness lain

| | Claude Code | Codex CLI | opencode |
|---|---|---|---|
| Sandbox OS macOS | Seatbelt via `/sandbox` (srt) | Seatbelt `/usr/bin/sandbox-exec` | Tidak ada |
| Linux | bubblewrap | bubblewrap + seccomp | Tidak ada |
| Scope tulis | cwd, temp, `--add-dir`, `sandbox.filesystem.allowWrite` | workspace (cwd, `/tmp`) atau read-only | Tidak ditegakkan |
| Network default | Proxy dengan `allowedDomains`/`deniedDomains` | Mati di `workspace-write`; `network_access = true` atau `features.network_proxy` | Tidak ditegakkan |
| Mode | `default`, `acceptEdits`, `plan`, `auto`, `dontAsk`, `bypassPermissions` | sandbox `read-only`/`workspace-write`/`danger-full-access` × approval `on-request`/`never`/granular | Rule allow/ask/deny |
| Rule perintah | `Bash(npm run test:*)`, urutan deny > ask > allow | execpolicy + approved prefixes | `{action:"shell", resource:"git status *", effect:"allow"}` (schema V2) |
| Perintah majemuk | Dipecah di `&&`, `\|\|`, `;`, `\|`, `&`, newline | Bypass sandbox hanya jika semua segmen cocok allow | Scanner memecah; `eval`/sintaks rusak fail-closed bila ada deny |
| Escape hatch | Retry `dangerouslyDisableSandbox` dengan prompt; matikan via `allowUnsandboxedCommands: false` | `danger-full-access` / `--yolo` | n/a |

Detail:
- Claude Code: deny rule berbasis teks "bukan batas keamanan" menurut docs; wrapper seperti `npx`, `docker exec` tidak di-strip, dan `Bash(curl *)` tidak menangkap `/usr/bin/curl` atau `sh -c 'curl …'`. Hook `PreToolUse` bisa mengembalikan `permissionDecision: "deny"`. https://code.claude.com/docs/en/sandboxing, https://code.claude.com/docs/en/permissions, https://code.claude.com/docs/en/hooks, https://github.com/anthropics/claude-code/issues/92542
- Codex: `.git` (termasuk target `gitdir:`), `.agents`, `.codex`, `.aws` tetap read-only di dalam writable root. `approval_policy = "untrusted"` sudah dipensiunkan. `approvals_reviewer = "auto_review"` meneruskan approval ke agent reviewer. https://learn.chatgpt.com/docs/agent-approvals-security, https://github.com/openai/codex/issues/37405
- opencode: docs menyebut shell berjalan dengan otoritas penuh user host. Request sandbox ditutup "not planned" (#21733), PR sandbox #52320 ditutup tanpa merge, #48411 masih terbuka. Base policy V2 dimulai dari allow `*`, jadi set rule `shell` eksplisit. https://opencode.ai/docs/permissions/, https://github.com/anomalyco/opencode/issues/21733

Pelajaran: rule teks hanya untuk UX (mengurangi prompt). Batas keamanannya adalah sandbox OS. Harness yang paling aman (Codex) menyalakan sandbox dan mematikan network secara default, lalu meminta approval untuk keluar dari sandbox.

## Rekomendasi

### Analyst (default, non-IT)
1. Pyodide 314.x dibundel di app (core + numpy + pandas + deps + python-calamine + wheel openpyxl, ~20 MB). Tidak ada unduhan saat runtime.
2. Jalankan di WKWebView tersembunyi terpisah, aset disajikan lewat custom scheme dari Go dengan CSP `default-src 'self'; connect-src 'none'` dan header COOP/COEP untuk `SharedArrayBuffer`.
3. File: Go membaca file yang dipilih user, kirim bytes ke worker, `FS.writeFile` ke `/in/`; hasil dari `/out/` dibaca `FS.readFile` dan dikembalikan ke Go. Tidak ada akses FS host dari Python.
4. Timeout: interrupt buffer untuk soft stop, `terminate()` untuk hard stop. Pre-warm satu worker saat idle (biaya ~0.5 GB RAM; lepaskan setelah idle N menit di mesin 8 GB).
5. Batasi ukuran input (heap wasm maks 4 GiB, praktis jauh lebih kecil di 8 GB).

### Coding (opt-in)
1. Shell host lewat `/usr/bin/sandbox-exec -f <profile>` (path absolut, profil di file). Profil `(deny default)` turunan Codex: tulis hanya workspace + temp, `.git` read-only, network mati.
2. Approval: rule allow/ask/deny per prefix perintah dengan pemecahan perintah majemuk, untuk mengurangi prompt. Keluar sandbox (network, path di luar workspace) selalu minta approval eksplisit; tidak ada mode "yolo" default.
3. Network opsional lewat proxy lokal di Go dengan allowlist domain, seperti srt/Codex.
4. Siapkan jalur cadangan karena `sandbox-exec` deprecated: Apple `container` di macOS 26+ untuk user yang mau isolasi VM dan punya RAM.

### Ditunda / pantau
- CPython WASI + wazero: sampai ada build numpy/pandas wasip1 yang dirawat.
- `maxMemoryBytes` Pyodide (belum rilis).

## Gap verifikasi
- Pyodide di WKWebView (RAM, cold start, CSP worker, COOP/COEP lewat custom scheme) belum diukur; angka di atas dari Node.
- Boot time dan RAM Apple `container`, libkrun, microsandbox.
- Ukuran unpacked wheel native numpy/pandas/openpyxl untuk mode Seatbelt.
- Status `sandbox-exec` di SDK/macOS mendatang tidak dipublikasikan Apple.
