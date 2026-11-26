# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

Aplikasi desktop macOS (MyGo + Svelte 5 di OS WebView). Perilaku jendela native macOS (traffic lights, menu, dialog). Windows dan Linux menyusul.

## Stack

Svelte 5 + TypeScript + Vite untuk frontend; Go untuk core (TAD). Prototype UI memakai Svelte 5 + Vite di `prototypes/ui/` (branch throwaway).

## Users

- **Utama:** pengguna non-IT dan knowledge worker yang bertanya, merangkum dan menganalisis file (Excel/CSV/PDF), menulis dokumen, dan ingin memahami konsep sulit tanpa paham MCP, shell, atau API provider.
- **Analis:** mengolah data, menghitung, membuat grafik dan laporan.
- **Power user / developer:** mengatur provider (OpenAI-compatible, Anthropic-compatible, lokal), MCP server, Mode, izin, dan diagnostik cache.
- **Pengguna sadar privasi:** memilih inferensi lokal dan ingin tahu kapan data keluar dari device.

## Product Purpose

Workspace AI desktop yang mengubah niat pengguna menjadi hasil kerja lewat percakapan, tools, agent, dan artifact visual. Sukses berarti pengguna memahami jawaban (bukan hanya membacanya), bisa melanjutkan pekerjaan nyata (file, laporan, analisis), dan tidak pernah terkejut oleh data yang terkirim atau biaya yang muncul.

## Positioning

- Local-first dan provider-agnostic: riwayat, artifact, dan pengaturan ada di komputer pengguna; pengguna memilih provider sendiri.
- Jawaban penjelasan konsep atau proses bersifat visual-first: artifact interaktif atau UI block inline, lalu teks paragraf di bawahnya.
- Jujur soal egress data dan biaya: operasi remote ditandai, peringatan reset cache saat ganti model atau Mode, usage bisa dilihat.

## Operating Context

- Navigasi berpusat pada chat: Chat baru, Cari, Projects, Artifacts, riwayat; pengaturan di Settings.
- Composer berisi chip Mode dan Model plus lampiran.
- Jawaban: ringkasan activity terlipat di atas, visual interaktif inline, teks di bawah, chip sumber untuk riset, kartu approval inline saat run berhenti.
- Artifact di-expand ke panel kanan; bisa dibuka di window terpisah.
- Pemakaian harian, sesi panjang, di Mac (termasuk 8 GB RAM).

## Capabilities and Constraints

- Mode: Chat, Riset, Analis Data, Penulisan, Coding.
- UI block (komponen milik Sasmita) dan sandboxed artifact (HTML/React di iframe sandbox, tanpa network).
- Maks 2 artifact hidup bersamaan, 1 WebGL; artifact di luar layar menjadi kartu.
- Target resource: idle ≤150 MB, chat ≤250 MB (seluruh app + WebView).
- Bahasa UI: Indonesia dan Inggris (bisa dipilih). Copy harus tahan perbedaan panjang teks antar bahasa.
- Belum diputuskan: provider default untuk pengguna tanpa API key.

## Brand Commitments

- Nama: **Sasmita**, terinspirasi makna isyarat, tanda, atau ungkapan niat.
- Istilah resmi di UI: Mode, artifact, project (lihat GLOSSARY.md).
- Referensi tingkat kualitas yang diberikan user: Claude dan ChatGPT desktop (tipografi lega, visual inline menyatu dengan teks, activity terlipat satu baris, chip sumber, composer berbentuk pill). Ini batas kualitas, bukan kewajiban meniru tampilan.
- Dark mode sebagai default; light mode harus tersedia.

## Evidence on Hand

- Belum ada logo, aset brand, screenshot produk, testimoni, atau data pengguna. Jangan mengarang klaim, angka pengguna, atau harga.
- Konten contoh boleh memakai skenario nyata dari diskusi: penjelasan flow training neural network, analisis workbook penjualan, riset perbandingan.

## Product Principles

1. Pahamkan, jangan hanya jawab: visual dipakai bila membantu memahami, teks tetap untuk hal sederhana.
2. Pengguna non-IT tidak boleh perlu memahami istilah teknis untuk menyelesaikan tugas.
3. Tidak ada kejutan: data yang keluar, aksi berisiko, dan biaya selalu terlihat sebelum terjadi.
4. Ringan secara default: fitur berat hanya hidup saat dipakai.

## Accessibility & Inclusion

- Dua bahasa (ID/EN). Approval dan error memakai bahasa biasa.
- Kontras dan fokus keyboard memadai di dark dan light mode.
