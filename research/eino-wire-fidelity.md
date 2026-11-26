# Fidelitas wire Eino untuk Anthropic dan OpenAI-compatible (status 26 Nov 2026)

Issue: #3 (bagian dari #1). Konteks: `prompt-caching.md` (rule set) dan `core-dependencies.md` (verdict Eino).

## Jawaban singkat
- **Claude (`eino-ext/components/model/claude`)**: tidak persis. Konversinya deterministik, jadi array yang sama selalu menghasilkan byte yang sama, tapi bentuknya berbeda dari array aplikasi: tool result dan system message berdekatan digabung, field JSON Schema selain `properties`/`required` dibuang, tool result kosong berubah jadi text block kosong tanpa `tool_use_id`, `redacted_thinking` dibuang. Breakpoint manual dan TTL 1h didukung. Tidak ada validasi batas 4 breakpoint.
- **OpenAI-compatible (`components/model/openai` → `libs/acl/openai`)**: message dikirim 1:1, tanpa merge. `prompt_cache_key` bisa lewat `ExtraFields`. `cached_tokens` terbaca, `prompt_cache_hit_tokens` (DeepSeek) tidak.
- **Rekomendasi**: adapter milik aplikasi yang mengimplementasikan `model.ToolCallingChatModel` untuk Anthropic. Untuk OpenAI-compatible, eino-ext masih bisa dipakai dengan `ExtraFields` + `ResponseMessageModifier`, tapi adapter yang sama memberi satu jalur serialisasi yang bisa diuji dengan golden test. ADK tetap dipakai karena hanya butuh `Generate`/`Stream` dan opsi `model.WithTools`.

## Versi yang diperiksa
- eino `v0.9.23` (commit `35480319`). Main per 9 Nov 2026 = v0.9.23 + 1 commit.
- eino-ext main `48998f2` (25 Nov 2026). Tag rilis terakhir: claude `v0.1.26`, openai `v0.1.13`, `libs/acl/openai` `v0.1.17`, openrouter `v0.1.10`, deepseek `v0.1.7`. Claude main beda dari `v0.1.26` hanya pada dukungan file ID (`2db7d27`). Bagian yang dikutip di bawah sama.
- SDK: `anthropic-sdk-go v1.56.0`, `meguminnnnnnnnn/go-openai` (fork sashabaranov), `evanphx/json-patch v0.5.2`.
- Context7 (`/cloudwego/eino-ext`) hanya mengembalikan README `agenticclaude`. Semua klaim di bawah berasal dari source.

Singkatan permalink:
- `EXT` = https://github.com/cloudwego/eino-ext/blob/48998f2f29341edb727da4e8ff4f94e1fe84c33f
- `EINO` = https://github.com/cloudwego/eino/blob/35480319ee8f6f75f612b4dce0bd5f4747b37730

## Probe
Probe dijalankan di luar repo (`/tmp/einoprobe`, Go 1.27.0): `httptest` server menangkap body request dari `claude.NewChatModel` dan `openai.NewChatModel` untuk history berikut: 2 system, user, assistant dengan 2 tool call (args `{"zeta":"1","alpha":"2"}` dan `{"alpha":"2", "zeta":"1"}`), tool result `"r1"`, tool result `""`, 2 system di tengah, user. Tool schema: `properties` urutan `zeta, alpha`, `additionalProperties:false`, `description` top-level.

Body Claude yang terkirim (dipotong):
```json
{"messages":[
 {"role":"user","content":[{"type":"text","text":"hi"}]},
 {"role":"assistant","content":[{"type":"tool_use","id":"c1","input":{"zeta":"1","alpha":"2"}},{"type":"tool_use","id":"c2","input":{"alpha":"2", "zeta":"1"}}]},
 {"role":"user","content":[{"type":"tool_result","tool_use_id":"c1","content":[{"type":"text","text":"r1"}]}]},
 {"role":"user","content":[{"type":"text","text":""}]},
 {"role":"system","content":[{"type":"text","text":"mid sys 1"},{"type":"text","text":"mid sys 2"}]},
 {"role":"user","content":[{"type":"text","text":"next","cache_control":{"ttl":"1h","type":"ephemeral"}}]}],
 "system":[{"text":"sys A"},{"text":"sys B","cache_control":{"ttl":"1h","type":"ephemeral"}}],
 "tools":[{"name":"t1","input_schema":{"properties":{"zeta":{},"alpha":{}},"required":["zeta","alpha"],"type":"object"},"cache_control":{"ttl":"1h","type":"ephemeral"}}]}
```
Body OpenAI: 9 message 1:1 dengan input; schema lengkap (`additionalProperties`, `description` ada), `required` jadi `["alpha","zeta"]`. Dengan `WithExtraFields({"prompt_cache_key":"sess-1"})`, key top-level body diurutkan alfabetis (`messages, model, prompt_cache_key, tools`), isi nested tidak berubah.

## Anthropic: `components/model/claude`

### Bentuk message
- System message di awal array masuk ke `system[]`. System message sesudah user pertama tetap di `messages` dengan role `system` (`EXT/components/model/claude/claude.go#L594-L611`, `#L1263-L1267`).
- `mergeAdjacentToolResults` menggabungkan user message yang seluruhnya `tool_result` ke satu message; `mergeAdjacentSystemMessages` menggabungkan system berurutan (`#L752-L753`, `#L766-L804`). API Anthropic memang mewajibkan semua `tool_result` satu turn dalam satu user message. Merge ini deterministik dan hanya bergantung pada input, jadi prefix tetap stabil selama history append-only.
- **Bug**: tool message dengan `Content == ""` dan tanpa multi-content tidak masuk cabang `tool_result` (`#L1199-L1204`). Hasilnya block kosong diganti `NewTextBlock("")` (`#L1252-L1256`) dan dikirim sebagai user message biasa. `tool_use_id` hilang (lihat probe). ADK membuat `ToolMessage(output, callID)` apa adanya (`EINO/compose/tool_node.go#L1233-L1240`), jadi tool yang mengembalikan string kosong akan memicu ini.
- Assistant message dibangun ulang dengan urutan tetap: `thinking` (hanya bila ada signature), tool_search events, teks/multi-content, lalu `tool_use` (`#L1060-L1077`, `#L1236-L1247`). Respons dengan beberapa thinking block atau text di antara tool_use tidak dipertahankan urutannya. `RedactedThinkingBlock` dibuang saat parsing respons (`#L1528`).

### Tool schema dan args
- `toAnthropicToolParam` hanya menyalin `Properties` dan `Required` (`#L545-L578`). `ToolInputSchemaParam` SDK punya `ExtraFields`, tapi tidak diisi, jadi `additionalProperties`, `$defs`, `description` top-level, `oneOf` di root, dll. hilang.
- Urutan `properties` mengikuti `orderedmap` (insertion order). `ParamsOneOf` dari `map[string]*ParameterInfo` diurutkan alfabetis (`EINO/schema/tool.go#L304-L365`). Keduanya deterministik.
- Args `tool_use` dikirim sebagai `json.RawMessage(args)` dari string `Arguments` (`#L1236-L1247`); probe menunjukkan spasi pun dipertahankan. Dari respons, input diambil sebagai string mentah (`#L1633-L1666`). `"{}"` dinormalisasi ke `""` lalu dikirim lagi sebagai `{}` (`#L1656`, `#L1238-L1241`). Byte stabil selama aplikasi menyimpan string args persis.

### `cache_control`
- Manual: `SetMessageCacheControl(msg, &CacheControl{TTL: CacheTTL1h})` dan `SetToolInfoCacheControl` menyimpan penanda di `Extra` (`EXT/components/model/claude/message_extra.go#L27-L100`). Breakpoint ditaruh di block terakhir message itu (`claude.go#L1249-L1251`, `#L1385-L1402`).
- Auto (`Config.AutoCacheControl` atau `WithAutoCacheControl`): satu breakpoint di system terakhir, satu di tool terakhir, satu di block terakhir message terakhir (`#L729-L732`, `#L901-L916`, `#L755-L759`). Masing-masing hanya dipasang bila grup itu belum punya breakpoint manual. Satu breakpoint manual di message mana pun mematikan auto breakpoint di message terakhir.
- TTL 1h: didukung (`CacheTTL1h`). TTL kosong = default 5m (`#L534-L543`).
- Tidak ada pengecekan batas 4 breakpoint maupun aturan urutan TTL (1h harus sebelum 5m). Kesalahan baru muncul sebagai error API.
- Tidak ada breakpoint otomatis "sebelum turn yang menambah banyak block" (rule 5 di `prompt-caching.md`). Itu harus manual.

### Usage
- `toTokenUsage`: `CachedTokens = cache_read_input_tokens`, `CacheWriteTokens = cache_creation_input_tokens`, `PromptTokens = input + read + creation` (`#L1405-L1441`). `cache_creation_input_tokens` juga disalin ke `Extra` (`message_extra.go#L225-L240`).
- Pecahan `cache_creation.ephemeral_5m_input_tokens` / `ephemeral_1h_input_tokens` tidak dibaca (probe: dikirim 20/30, tidak muncul).
- Stream: `ConcatMessages` mengambil nilai maksimum per field usage, jadi angka dari `message_start` dan `message_delta` tidak dijumlah dua kali (`EINO/schema/message.go#L1761-L1787`).

### Escape hatch
Hanya `Config.HTTPClient` (`claude.go#L133-L135`) dan `AdditionalRequestFields` (statis per instance, `#L141-L143`). Tidak ada request-body modifier per call. Memperbaiki bug di atas lewat `RoundTripper` berarti mem-parse dan menulis ulang body JSON, yang sama beratnya dengan adapter sendiri.

## OpenAI-compatible: `components/model/openai` → `libs/acl/openai`

### Bentuk message
- Satu `schema.Message` → satu `ChatCompletionMessage`, tanpa merge (`EXT/libs/acl/openai/chat_model.go#L689-L723`). `ReasoningContent` ikut dikirim sebagai `reasoning_content` (`#L713`).
- Tool message selalu menyertakan `content` walau kosong (fork go-openai `chat.go#L184-L198`).
- Args tool call dikirim sebagai string apa adanya (`#L370-L392`).

### Tool schema
- Schema utuh dari `ToJSONSchema()`. `toTools` mengurutkan `required` secara alfabetis dan rekursif ke properti (`#L1284-L1336`). Mutasi ini terjadi di objek schema milik pemanggil bila schema dibangun dari `*jsonschema.Schema`.

### `prompt_cache_key` / `session_id`
- Tidak ada field khusus di fork go-openai. Pakai `Config.ExtraFields` atau `openai.WithExtraFields` per call (`EXT/components/model/openai/option.go#L36-L56`, `acl chat_model.go#L590-L596`, `#L642-L644`).
- Bila `ExtraFields` dipakai, fork memakai `jsonpatch.MergePatch`, yang men-decode top-level ke map lalu marshal ulang: key top-level jadi alfabetis, nilai nested tetap byte mentah (`go-openai internal/marshaller.go#L16-L43`, `json-patch merge.go#L103-L162`). Hasilnya tetap deterministik.
- `RequestPayloadModifier` memberi akses ke raw body per call (`#L741-L745`).
- **OpenRouter component**: `buildOptions` hanya meneruskan model, max tokens, top_p, stop, tools, tool_choice (`EXT/components/model/openrouter/chatmodel.go#L391-L438`). `WithExtraFields`, temperature, dan opsi lain per call dibuang. `session_id`/`prompt_cache_key` hanya bisa lewat `Config.ExtraFields`, artinya satu instance ChatModel per session. Komponen ini mendukung `cache_control` per message/part dengan TTL 1h via sjson (`#L226-L320`, `message_extra.go#L155-L250`).

### Usage
- `cached_tokens` dan `cache_write_tokens` dari `prompt_tokens_details` dibaca (`#L1338-L1360`). Stream memakai `include_usage: true` (`#L881`).
- `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` (DeepSeek via endpoint OpenAI-compatible) masuk ke `Usage.ExtraFields` di fork tapi tidak dipetakan dan tidak sampai ke `schema.Message` (probe: `CachedTokens` = 64 dari `cached_tokens`, field 80/20 hilang). Bisa diambil lewat `WithResponseMessageModifier` / `WithResponseChunkMessageModifier` yang menerima raw body (`#L779-L781`, `#L967-L970`).
- Komponen `deepseek` (client `cohesion-org/deepseek-go`) memetakan `prompt_cache_hit_tokens` ke `CachedTokens`, tanpa miss tokens (`EXT/components/model/deepseek/deepseek.go#L893-L919`).

## Sisi ADK yang memengaruhi cache
- `ChatModelAgentConfig.Model` bertipe `model.BaseModel[M]`; tool diberikan per call lewat `model.WithTools` (`EINO/adk/chatmodel.go#L277-L280`, `#L1482`, `#L1571`, `EINO/adk/wrappers.go#L1255-L1258`). Urutan tool = urutan `ToolsConfig.Tools` (`EINO/adk/react.go#L312-L324`).
- `defaultGenModelInput` memformat `Instruction` sebagai FString dengan SessionValues setiap call (`EINO/adk/chatmodel.go#L167-L193`). Placeholder seperti `{Time}` membuat system prompt berubah tiap request. Pakai `GenModelInput` sendiri.
- Komentar Eino sendiri: mengubah tools di `WrapModel` "break prompt cache" (`EINO/adk/chatmodel.go#L399-L400`).
- Hasil tool paralel diurutkan berdasarkan index tool call, bukan urutan selesai (`EINO/compose/tool_node.go#L1233-L1240`).

## Opsi adapter milik aplikasi
ADK hanya butuh `Generate`/`Stream` (`EINO/components/model/interface.go#L36-L42`, `#L71`); `ToolCallingChatModel` menambah `WithTools` (`#L99-L103`). Adapter di belakang `ModelProvider`:

```go
type AnthropicModel struct{ cli anthropic.Client; tools []*schema.ToolInfo; cfg Config }

func (m *AnthropicModel) Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error) {
    co := model.GetCommonOptions(&model.Options{Tools: m.tools}, opts...)   // ADK kirim tools di sini
    so := model.GetImplSpecificOptions(&wireOpts{}, opts...)                 // breakpoint, TTL, session key
    req, err := buildRequest(in, co, so)  // satu serializer: tanpa drop field, tool_result kosong tetap tool_result,
                                          // breakpoint eksplisit (maks 4, 1h sebelum 5m), args sebagai RawMessage
    if err != nil { return nil, err }
    resp, err := m.cli.Messages.New(ctx, req)
    if err != nil { return nil, err }
    return toSchema(resp) // isi ResponseMeta.Usage (CachedTokens, CacheWriteTokens) + Extra untuk 5m/1h split,
                          // simpan block asli (thinking/redacted) di Extra agar bisa di-replay persis
}
func (m *AnthropicModel) Stream(...) (*schema.StreamReader[*schema.Message], error) // chunk dengan ToolCall.Index agar ConcatMessages benar
func (m *AnthropicModel) WithTools(t []*schema.ToolInfo) (model.ToolCallingChatModel, error) { c := *m; c.tools = t; return &c, nil }
func (m *AnthropicModel) IsCallbacksEnabled() bool { return false } // ADK menyuntik callback (adk/chatmodel.go#L332)
```

Trade-off:

| | eino-ext claude/openai | Adapter sendiri |
|---|---|---|
| Fidelitas wire | Claude: merge, drop schema, bug tool result kosong. OpenAI: 1:1 | Penuh, bisa golden test byte |
| Breakpoint/TTL | Auto 3 breakpoint atau manual via `Extra`, tanpa validasi | Kebijakan sendiri (rule 5), divalidasi |
| Usage cache | Claude read/write; OpenAI `cached_tokens`; DeepSeek hit hilang | Semua field, termasuk 5m/1h dan hit/miss |
| Biaya | Gratis, ikut upgrade upstream | ~500–800 baris per provider (perkiraan), wajib ikuti perubahan API |
| ADK | Ya | Ya, interface sama |

Alternatif yang belum diperiksa: `components/model/agenticclaude` bekerja di level content block (`schema.AgenticMessage`) dan punya `SetContentBlockCacheControl`. ADK generik terhadap `M`, jadi ini bisa jadi jalan tengah. Perlu dicek apakah ia juga membuang field schema dan menangani tool result kosong.

## Belum diverifikasi
- Apakah API Anthropic menerima role `system` di dalam `messages` (SDK v1.56 punya `MessageParamRoleSystem`, tapi tidak dicek ke docs atau API).
- Probe memakai mock server, bukan API sungguhan. Respons Anthropic terhadap tool result kosong (kemungkinan 400) belum dicoba.
- Isi `agenticclaude` dan `agenticopenai`.
