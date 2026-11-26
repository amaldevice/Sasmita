# Prompt caching (status 25 Nov 2026)

## Mekanisme per provider
- Anthropic: urutan prefix tools -> system -> messages. Maks 4 breakpoint `cache_control`. Minimum 512–4,096 token tergantung model. Lookback 20 block. Write 1.25x (5m) / 2x (1h), read 0.1x. Perubahan tool definition membatalkan semuanya; perubahan `tool_choice`, image, thinking config, effort membatalkan block message. https://platform.claude.com/docs/en/build-with-claude/prompt-caching
- OpenAI: otomatis, minimum 1,024 token, `prompt_cache_key` untuk routing. https://developers.openai.com/api/docs/guides/prompt-caching
- Gemini: implicit caching 2.5+ (min 2,048; 4,096 di 3.x/4). Gemini 4 hanya reuse request sebelumnya bila menambah >= 2,050 token. Explicit cache TTL default 1 jam, tidak tersedia di Gemini 4. https://aistudio.google.com/docs/caching
- DeepSeek: disk cache otomatis, hit butuh full match prefix unit yang sudah persisted. https://api-docs.deepseek.com/guides/kv_cache
- OpenRouter: sticky routing 60 menit via `session_id` / `prompt_cache_key`. `provider.order` mematikannya. https://openrouter.ai/docs/guides/best-practices/prompt-caching
- llama.cpp / Ollama: exact token-prefix match terhadap slot yang resident. `cache_prompt` default on. Ollama unload model setelah 5 menit default.

## Field usage
Anthropic `cache_read_input_tokens`, `cache_creation_input_tokens`; OpenAI `input_tokens_details.cached_tokens`, `cache_write_tokens`; Gemini `usage_metadata` cached count; DeepSeek `prompt_cache_hit_tokens`, `prompt_cache_miss_tokens`; llama.cpp `timings.cache_n`.

## Anti-pattern (bukti dari judul/cuplikan issue)
1. Timestamp di system prompt: pi #1873, opencode #29672 (date rollover, git/workspace field berubah saat resume).
2. Breakpoint di block yang berubah-ubah (Anthropic docs).
3. Satu turn menambah >20 block: claude-code #111163.
4. Menulis ulang history: claude-code #48734, #83913.
5. Tool list / TTL berubah mid-session: claude-code #81967.
6. State per-step di depan system prompt: opencode-orchestrator #57 (hit rate 4.15%).
7. Tidak ada session key: pi #3196, opencode #43689.
8. Compaction mengubah prefix (OpenAI docs).
9. Idle > 5 menit: opencode #16848.
10. Ganti provider/model: opencode #26460.
11. Warm-up request yang tidak sama dengan request asli: opencode #62401.
12. Request paralel sebelum respons pertama (Anthropic docs).
13. Urutan key JSON tidak stabil di `tool_use` (Anthropic docs).

## Rule set yang direkomendasikan
1. Bekukan tools per session (nama, deskripsi, schema, byte serialisasi). Batasi via `tool_choice`/`allowed_tools`/deferred loading.
2. System prompt statis. Jam, cwd, git state masuk sebagai tail message.
3. Message log append-only. Compaction jarang; anggarkan satu miss per compaction.
4. Aturan, reminder, perubahan env ditambahkan sebagai tail message baru, bukan re-render history.
5. Anthropic: breakpoint di block stabil terakhir, satu lagi sebelum turn yang menambah banyak block. TTL 1h bila idle gap > 5 menit.
6. Satu model per session, pin session key.
7. Serialisasi JSON deterministik.
8. Log read/write/miss per request, alert bila turun.
9. Keepalive hanya saat user aktif. Ping entry 5m selama 1 jam ~2.35x prefix vs 2x untuk satu write 1h.
