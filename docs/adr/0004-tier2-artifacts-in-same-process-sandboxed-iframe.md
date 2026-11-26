# Sandboxed artifact di iframe satu proses lewat `artifact://`, bukan srcdoc atau WebView terpisah

Sandboxed artifact (Tier 2) dirender inline di chat seperti Claude. MyGo v0.10 hanya bisa memberi WebView terpisah lewat window terpisah, dan site isolation WebKit masih mati, sehingga iframe berbagi proses dan main thread dengan UI chat (research #4). Kami memutuskan iframe `sandbox="allow-scripts"` yang memuat `artifact://<id>/` dari handler Go dengan header CSP `sandbox allow-scripts` dan `connect-src 'none'`; `srcdoc` ditolak karena mewarisi CSP UI trusted. Risiko hang diterima dan dimitigasi watchdog Go (ping frontend, reload + nonaktifkan artifact bila main thread tidak merespons ~3 detik) serta opsi "Buka di window terpisah" memakai window `Untrusted`.

## Consequences

- Main window wajib menolak navigasi main frame ke `artifact://` (custom scheme dianggap trusted oleh MyGo di main frame).
- Handler custom scheme tidak boleh punya efek samping; semua aksi lewat bridge postMessage yang di-whitelist host.
- Ini bukan pertahanan terhadap exploit renderer; bila threat model berubah, pindah ke WebView/proses terpisah.
