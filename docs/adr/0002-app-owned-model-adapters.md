# Adapter model milik aplikasi, Eino hanya untuk ADK

Komponen model eino-ext me-reshape message Anthropic, membuang sebagian tool schema, tidak membaca field cache DeepSeek, dan komponen OpenRouter membuang extra fields per-call (research ticket #3). Karena invarian byte-stable prefix (ADR-0001) harus dijamin dan mayoritas pemakaian adalah provider OpenAI-compatible, kami menulis adapter sendiri untuk Anthropic dan OpenAI-compatible yang mengimplementasikan interface `ToolCallingChatModel` Eino, sehingga ADK (loop, interrupt/resume) tetap dipakai tanpa bergantung pada serialisasi library.

## Considered Options

- eino-ext + response modifier untuk OpenAI-compatible: lebih sedikit kode, tapi invarian serialisasi bergantung pada perilaku library yang bisa berubah antar versi.
