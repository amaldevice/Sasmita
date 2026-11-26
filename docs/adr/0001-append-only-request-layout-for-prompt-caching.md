# Append-only request layout demi prompt caching

TAD v0.2.0 menyusun ulang context tiap langkah dan memakai sliding window, yang menggeser prefix setiap turn dan membuat cache hit mendekati 0% (pola yang sama dengan masalah di opencode dan pi). Kami memutuskan request selalu berbentuk `[tools beku per conversation][system statis][history append-only byte-identik][user turn baru + context note]`; informasi dinamis hanya masuk lewat context note di user turn (isi delta saja), dan prefix hanya boleh berubah lewat compaction (trigger ~75% input budget, sisakan 2 user turn terakhir verbatim) atau ganti model yang diperingatkan ke user. Konsekuensinya active context tidak lagi "dipilih ulang" per langkah seperti §11.3–11.4 TAD; relevansi diatur lewat apa yang ditambahkan, bukan apa yang dibuang.

## Consequences

- Golden test: request turn N yang terserialisasi harus jadi prefix byte-exact dari request turn N+1.
- Tool baru tidak boleh mengubah daftar tools di tengah conversation (lihat ticket tool discovery).
- Isi skill dimuat lewat tool result, bukan system prompt; instruksi tanpa placeholder per-request.
