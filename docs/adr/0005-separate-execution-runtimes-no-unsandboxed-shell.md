# Tiga runtime eksekusi terpisah, tanpa shell host tak-tersandbox

Sasmita dipakai pengguna non-IT di Mac 8 GB, dan rule teks perintah bukan batas keamanan (research #5). Kami memutuskan tiga runtime yang tidak saling memanggil: QuickJS-WASM di Go untuk `run_script` (orkestrasi tool, tanpa FS), Pyodide di window `Untrusted` tersembunyi untuk analisis data (tanpa network dan FS host, file disalin sebagai bytes, output jadi artifact, tanpa approval per run, lazy start dan mati setelah 10 menit idle), dan shell host hanya di harness Coding di bawah `/usr/bin/sandbox-exec` (`deny default`, tulis hanya workspace + temp, `.git` read-only, network mati, keluar sandbox selalu butuh approval). Bila `sandbox-exec` tidak tersedia, shell dimatikan, bukan dijalankan tanpa sandbox.

## Considered Options

- Python native di host (Seatbelt): semua library tersedia, tetapi butuh distribusi CPython + wheel native; ditunda sampai Pyodide kurang.
- VM/container (Apple `container`, Docker): isolasi terkuat, tetapi ~1–2 GiB per VM; hanya opsi isolasi kuat di masa depan.
- Shell host dengan approval saja (pola opencode): ditolak karena approval bukan batas keamanan bagi pengguna non-IT.
