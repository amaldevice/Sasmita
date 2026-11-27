# Sasmita

**Local-first** workspace AI desktop untuk macOS Apple Silicon. Percakapan,
jawaban visual, dan tool agent berjalan dalam bahasa Indonesia atau Inggris.
Riwayat, artifact, dan pengaturan tetap di komputer kamu; provider dipilih
sendiri.

Status: **early preview**. Rilis dan catatan terbaru ada di
[GitHub Releases](https://github.com/amaldevice/Sasmita/releases/latest).

## Pasang

Unduh DMG dari [Releases](https://github.com/amaldevice/Sasmita/releases/latest).
Rilis ditandai *pre-release* dan app ditandatangani ad-hoc.

1. Buka DMG, pindahkan `Sasmita.app` ke **Applications**.
2. Klik kanan → **Open** → **Open** sekali saat pertama kali. Selesai ketika
   layar welcome muncul.
3. Jika macOS menolak:
   `xattr -dr com.apple.quarantine /Applications/Sasmita.app`
4. Opsional: `shasum -a 256 ~/Downloads/*.dmg`, lalu cocokkan dengan
   `SHA256SUMS.txt` di halaman rilis.

Pembaruan dilakukan manual dari halaman Releases.

## Build dari sumber

Butuh macOS 12+ (Apple Silicon), Go 1.27.3, Node.js 22+. Versi di-pin di
`go.mod` dan `shell/mygo/go.mod`.

```sh
git clone https://github.com/amaldevice/Sasmita.git
cd Sasmita
npm ci
go install github.com/egoist/mygo/cmd/mygo@v0.10.0
export PATH="$(go env GOPATH)/bin:$PATH"
cd shell/mygo && mygo build
```

Selesai ketika `build/darwin-arm64/` berisi `sasmita-*.dmg` dan `hdiutil verify`
pada DMG itu mengembalikan `checksum VALID`. Catatan build teruji ada di
[`docs/benchmarks/package-build-final.md`](docs/benchmarks/package-build-final.md).

## Verifikasi perubahan

`package.json` adalah sumber perintah. Gate CI menjalankan ini pada setiap PR:

```sh
npm run check
npx playwright install --with-deps chromium
npm run test:e2e
go test -count=1 ./...
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...
```

Selesai ketika kelima perintah exit 0. Untuk bereksperimen dengan UI, jalankan
`npm run dev`.

## Data dan privasi

- SQLite: `~/Library/Application Support/Sasmita/sasmita.db`, artifact di
  folder `artifacts/` di sebelahnya. Override dengan `SASMITA_DB_PATH`.
- API key disimpan di macOS Keychain, bukan di SQLite atau frontend.
- Permintaan ke provider remote ditandai di UI; tool berisiko meminta
  persetujuan sebelum dijalankan.

## Cakupan rilis ini

- Tersedia: chat streaming, Mode Chat/Riset/Penulisan, adapter
  OpenAI-compatible dan Anthropic-compatible, UI block, artifact sandbox,
  approval, compaction, Bahasa Indonesia/Inggris, dark/light.
- Dibutuhkan: endpoint OpenAI-compatible/Anthropic-compatible atau model lokal.
- Tahap berikutnya: MCP, Analis Data untuk Excel/CSV/PDF, long-form,
  Windows/Linux, auto-update. Lihat [TAD §22](SASMITA_TAD_v0.3.0.md).
- Isolasi artifact di WKWebView dan target memori masih checklist manual pada
  packaged app; buktinya di [`docs/benchmarks/`](docs/benchmarks/).

## Peta dokumen

- `PRODUCT.md` — tujuan, pengguna, prinsip. Baca saat memutuskan apa yang pantas dibangun.
- `SASMITA_TAD_v0.3.0.md` dan `docs/adr/` — desain dan alasan keputusan. Baca saat mengubah arsitektur.
- `docs/agents/` — konvensi issue, triage, domain. Baca saat berkontribusi atau menjalankan agent.
- `THIRD_PARTY_NOTICES.md` — lisensi dependency. Baca saat meninjau kewajiban distribusi.
- `LICENSE` — MIT untuk kode Sasmita.
