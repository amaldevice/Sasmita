# Design

Hasil ticket #12 (map #1). Sumber: prototype di branch `prototype/ui-design` (`prototypes/ui`), referensi Claude, ChatGPT, Parallel Chat API, Cohere, OpenAI Playground.

## World

**Standar kategori, dieksekusi rapi.** Aplikasi chat AI desktop yang familier (Claude/ChatGPT), dengan panel terpisah di atas latar (Cohere/Parallel) dan data dalam monospace (Playground). Kekhasan Sasmita datang dari jawaban visual-first, bukan dari kostum.

## Tema

- Default **gelap**. Terang tersedia. Token sama, nilai beda.
- Warna punya tugas, bukan dekorasi.

| Token | Gelap | Terang | Tugas |
|---|---|---|---|
| `--ground` | #141414 | #EDECE8 | Latar di belakang panel |
| `--bg` / `--side` | #1C1C1C | #FBFBFA | Panel sidebar, chat, panel artifact |
| `--surface` | #242424 | #FFFFFF | Composer, kartu, menu |
| `--raise` | #2B2B2B | #F1F0EC | Hover, item aktif, bubble user |
| `--line` | #333331 | #E3E1DB | Border 1px |
| `--text` | #EDEDEB | #1B1B1A | Teks utama, tombol primer (ink) |
| `--mute` | #9A9994 | #67665F | Teks sekunder, label |
| `--accent` / `--flow` | #7AA7FF | #2457D6 | Data, visual, kontrol aktif (bukan tombol primer) |
| `--attn` | #F08A4B | #C2410C | Hal yang menunggu pengguna (approval, peringatan) |
| `--ok` | #4ADE80 | #15803D | Selesai, artifact berjalan, cache hit |

## Tipografi

- UI dan teks: font sistem macOS (SF Pro Text / Display). Satu ukuran badan 16,5px, line-height 1,6, maks ~70ch.
- Monospace (SF Mono) **hanya** untuk data dan ukuran: baris usage, nama tool, label section kecil berhuruf kapital ("TERBARU"). Tidak untuk tombol.
- Judul jawaban 30px/700 (mode Chat), 36–50px (mode Baca). Hierarki lewat ukuran dan ketebalan.

## Bentuk dan layout

- Panel terpisah: gap 8px, radius 12px, border 1px. Radius kontrol 6px, kartu/menu 10px, composer 16px. Tidak ada pill di mana-mana.
- Sidebar 272px, bisa diciutkan ke rail 64px (tombol di header, ⌘\). Default terbuka. Mode Baca otomatis memakai rail.
- Kolom chat maks 760px; mode Baca 1040px.
- Artifact: bar kecil (status berjalan, Format, Buka di samping). Panel samping bisa layar penuh (Esc keluar).
- Format visual + teks: **1 kolom** (default mode Chat), **2 kolom bawah** (default mode Baca), **Kiri dan kanan**.

## Komponen kunci

- Activity: satu baris terlipat ("Bekerja 5 dtk ›"), dibuka jadi timeline dengan chip nama tool.
- Visual: label langsung di diagram (tanpa legenda kotak warna), ringkasan satu kalimat (tanpa baris angka besar), langkah bernomor hanya bila urutan bermakna.
- Usage (opsional, setting "selalu tampilkan"): monospace, `waktu ↑ input ↓ output % cache`.
- Empty state: judul + kalimat + 4 kartu saran 2×2.
- Approval: kartu bergaris `--attn` yang berdenyut sampai ditangani; tombol primer ink.

## Gerak

- Satu momen utama: jawaban tiba bertahap (activity → skeleton → visual menggambar diri → teks mengalir).
- Lainnya hanya umpan balik: 120–300ms, `cubic-bezier(0.16, 1, 0.3, 1)`. Loop (status berjalan, menunggu approval) berhenti bila off-screen.
- `prefers-reduced-motion`: langsung ke keadaan akhir.

## Dilarang (anti-slop)

Baris angka besar + label kecil, legenda kotak warna, satu kata diberi aksen/garis bawah, pill di semua kontrol, string meta dengan titik tengah, eyebrow di atas judul, font default AI (Inter, Plus Jakarta Sans) dan krem + serif, gradien teks, glass dekoratif, glyph/emoji sebagai ikon.

## Bahasa

UI dua bahasa (ID/EN). Angka mengikuti locale (0,11 / 0.11). Copy harus tahan teks yang lebih panjang.
