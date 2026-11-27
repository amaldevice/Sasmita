# Web search dengan rantai fallback BYOK, bukan key bersama

TAD §24 membuka keputusan provider web search, dan Sasmita local-first serta tidak memungut biaya. Kami memutuskan satu port `WebSearch` dengan empat adapter yang dipilih dan dipunya pengguna sendiri (kunci di macOS Keychain, SQLite hanya menyimpan reference), diurutkan **TinySearch → Tavily → Exa → Parallel**. TinySearch jadi primary karena TinySearch/TinyFetch benar-benar gratis tanpa kartu pada 30 request/menit dan 500 request/jam (~12.000/hari); Tavily memberi 1.000 credit/bulan, Exa $10 credit/bulan (~2.500 Lite search), dan Parallel $5/bulan dengan kartu atau MCP Launch tanpa akun. Rantai dipakai hanya saat Mode Riset atau tool eksplisit, mengirim query pengguna tanpa file/project, dan menampilkan label egress plus source chip. Setiap hasil dinormalisasi jadi URL, judul, excerpt, timestamp, dan provider, lalu perlakukan sebagai input tidak tepercaya sesuai TAD §11.6 dan §15. Kegagalan `401/403` menonaktifkan provider, `402/429` memberi cooldown sampai kuota reset, `5xx`/timeout/JSON rusak lanjut ke provider berikutnya; bila semua gagal, jawaban tetap diberikan dengan label "web search unavailable" dan tanpa sumber rekaan.

## Considered Options

- Satu key aplikasi bersama lewat MCP hosted seperti pola opencode: praktis, tetapi kuota dan ToS melekat pada akun bersama; ditolak untuk distribusi OSS.
- Google Custom Search JSON API: 100 query/hari gratis, tapi ditutup untuk pelanggan baru dan berhenti 1 Januari 2027; ditolak.
- Brave sebagai primary: sekarang $5 credit/bulan atau ~1.000 query, bukan 2.000 seperti dulu; layak sebagai fallback tambahan.
- Serper sebagai primary: 2.500 query sekali daftar, bukan kuota bulanan; cocok sebagai fallback ketiga atau keempat.
- SearXNG self-host: gratis dan privat, tetapi butuh server yang dikelola dan JSON API yang sengaja diaktifkan; ditunda sampai ada operator.
- Scraping HTML Google/Bing/DuckDuckGo: rapuh dan berisiko melanggar ToS; ditolak.
- Exa MCP / Parallel Launch tanpa key: menarik, tapi dukungan MCP masuk Phase 4; untuk v1 memakai adapter HTTP dengan key pengguna.
