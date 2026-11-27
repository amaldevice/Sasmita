# Sasmita

Workspace AI desktop yang local-first: percakapan, artifact visual, dan tool
agent dengan jawaban dalam bahasa Indonesia atau Inggris. Riwayat dan
pengaturan tetap di komputer kamu; kamu memilih sendiri provider modelnya.

Status: **early preview (v0.1.0)**. Lihat [PRODUCT.md](PRODUCT.md) untuk tujuan
produk dan [SASMITA_TAD_v0.3.0.md](SASMITA_TAD_v0.3.0.md) untuk desain teknis.

## Unduh

Unduh DMG untuk macOS Apple Silicon dari
[GitHub Releases](https://github.com/amaldevice/Sasmita/releases/latest).
Rilis saat ini ditandai sebagai *pre-release* dan app ditandatangani secara
ad-hoc, bukan dengan Developer ID.

### Memasang DMG ad-hoc

1. Buka DMG, lalu pindahkan `Sasmita.app` ke folder **Applications**.
2. Klik kanan `Sasmita.app` → **Open** → **Open** sekali pada peluncuran
   pertama. Setelah itu app bisa dibuka seperti biasa.
3. Alternatif lewat terminal, jika macOS menolak:

   ```sh
   xattr -dr com.apple.quarantine /Applications/Sasmita.app
   ```

4. Opsional, cocokkan checksum unduhan dengan `SHA256SUMS.txt` di halaman
   rilis:

   ```sh
   shasum -a 256 ~/Downloads/sasmita-0.1.0-arm64.dmg
   ```

Belum ada auto-update; cek halaman Releases untuk versi baru.

## Build dari sumber

Butuh macOS 12+, Apple Silicon, Go 1.27.3, dan Node.js 22+.

```sh
git clone https://github.com/amaldevice/Sasmita.git
cd Sasmita
npm ci
go install github.com/egoist/mygo/cmd/mygo@v0.10.0
export PATH="$(go env GOPATH)/bin:$PATH"
cd shell/mygo
mygo build
```

`mygo build` membangun frontend dan menghasilkan `Sasmita.app` serta DMG di
`build/darwin-arm64/`. Versi MyGo dan toolchain Go sengaja di-pin; lihat
[`docs/benchmarks/package-build-final.md`](docs/benchmarks/package-build-final.md)
untuk detail build yang sudah diuji.

## Pengembangan

```sh
npm run dev          # Vite dev server
npm run check        # svelte-check
npx playwright install chromium
npm run test:e2e     # 51 tes UI dengan IPC palsu
go test -count=1 ./...
```

## Data dan privasi

- Riwayat, project, dan artifact disimpan lokal di
  `~/Library/Application Support/Sasmita/sasmita.db` dan folder `artifacts/`
  di sebelahnya. Path bisa dioverride lewat `SASMITA_DB_PATH`.
- API key disimpan di macOS Keychain, bukan di SQLite atau frontend.
- Permintaan ke provider remote ditandai di UI; modes dan tool yang berisiko
  meminta persetujuan sebelum dijalankan.

## Batasan saat ini

- Perlu provider sendiri: endpoint OpenAI-compatible, Anthropic-compatible,
  atau model lokal. Belum ada provider default.
- Build yang tersedia hanya untuk macOS arm64. Windows, Linux, dan
  notarisasi Developer ID belum tersedia.
- MCP, Mode Analis Data untuk Excel/CSV/PDF, long-form, dan auto-update masih
  dalam tahap berikutnya.
- Artifact berjalan di iframe sandbox; verifikasi isolasi pada WKWebView
  hasil build akhir masih berlanjut (lihat `docs/benchmarks/`).

## Lisensi

MIT — lihat [LICENSE](LICENSE). Dependensi pihak ketiga dan lisensinya ada di
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
