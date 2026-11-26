# Tool discovery lewat meta-tool, bukan mengubah daftar tools

Daftar tools harus beku per conversation (ADR-0001), dan mayoritas pemakaian lewat provider OpenAI-compatible yang tidak punya deferred loading/tool search seperti Anthropic. Kami memutuskan: toolset inti harness dikirim native, ditambah meta-tool tetap `find_tools(query)` (FTS lokal, top 5 lengkap dengan schema, ≤8 KB) dan `call_tool(name, args)` untuk tool aktif lain mana pun; policy dan approval dievaluasi pada tool ID asli, dan validasi schema `call_tool` sepenuhnya di Go karena tidak bisa memakai strict decoding. Tool hasil discovery dipromosikan jadi native hanya saat compaction.

## Considered Options

- Semua tool aktif native: sederhana tapi ribuan token schema per request dan akurasi pemilihan tool turun.
- Tool search native Anthropic: dua jalur kode, tidak tersedia di OpenAI-compatible.
- Code Mode (`run_script`, QuickJS-WASM via wazero) sebagai pengganti: hemat untuk fan-out tetapi vendor hanya mengaktifkannya untuk model kuat. Diputuskan sebagai pelengkap bertahap, per model, mati secara default, memakai dispatcher yang sama dengan `call_tool`.
