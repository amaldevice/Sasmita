# Glossary

**Canonical history**: Seluruh pesan dan event tool sebuah conversation, disimpan permanen. Tidak pernah diubah setelah ditulis.
_Avoid_: memory, context

**Active context**: Isi yang dikirim ke model untuk satu langkah. Turunan dari canonical history, bukan sumber kebenaran.
_Avoid_: history, prompt

**Context note**: Informasi dinamis (pergantian tanggal, perubahan workspace, lampiran, memory hasil retrieval) yang ditempel ke user turn, berisi hanya apa yang berubah sejak note sebelumnya. Setelah terkirim menjadi bagian canonical history dan tidak diubah.
_Avoid_: system reminder, injected context

**Mode**: Lingkungan kerja sebuah run: instruksi, tools, skills, workspace, izin, dan kebijakan context, misalnya Mode Chat, Riset, Analis Data, Penulisan, Coding. Model hanya salah satu field-nya.
_Avoid_: harness, harness profile, agent, asisten

**UI block** (Tier 1): Visual inline yang dideskripsikan sebagai data deklaratif dan dirender oleh komponen milik Sasmita.
_Avoid_: widget, GenUI component

**Sandboxed artifact** (Tier 2): Kode HTML/JS buatan model yang dijalankan terisolasi dari aplikasi, tanpa akses ke data atau kemampuan desktop.
_Avoid_: preview, app, canvas

**Artifact runtime**: Kumpulan library berversi tetap yang boleh dipakai sandboxed artifact. Setiap artifact mencatat versi runtime-nya.
_Avoid_: CDN, dependencies

**Compaction**: Penggantian bagian lama canonical history di active context dengan ringkasan terstruktur, dipicu saat mendekati batas budget.
_Avoid_: truncation, sliding window

**Approval**: Persetujuan pengguna untuk satu panggilan tool berisiko. Terikat pada hash argumen yang persis dan snapshot policy; mengubah argumen membatalkannya. Bukan izin umum untuk tool tersebut.
_Avoid_: permission, izin tool selamanya

**Policy snapshot**: Versi dari aturan policy (TAD §15.3) yang disimpan bersama approval. Perubahan policy membuat approval dan grant lama tidak berlaku.
_Avoid_: config, setting

**Checkpoint**: Rekaman state run di SQLite saat run dijeda menunggu approval. Memungkinkan run dilanjutkan setelah aplikasi dimulai ulang tanpa meminta persetujuan yang sama dua kali.
_Avoid_: cache, session

