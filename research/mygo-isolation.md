# Isolasi WebView MyGo untuk Tier 2 artifact (status 26 Nov 2026)

Issue: #4 (bagian dari #1). Pertanyaan: bisakah MyGo v0.10.x (WKWebView di macOS) merender HTML/JS buatan model secara terisolasi dari app?

Sumber utama: source MyGo tag `v0.10.0` (commit `ebde939`, 2026-11-17, satu-satunya rilis v0.10.x), dibandingkan dengan `main` `96dd304` (2026-11-25; tidak ada perubahan yang relevan untuk isolasi). Docs di repo (`docs/`) adalah sumber `mygo.run/docs`. Context7 punya `/egoist/mygo` v0.10.0, tetapi klaim di bawah dibaca langsung dari source. Sisi WebKit: header API dan `UnifiedWebPreferences.yaml` di WebKit `main` (2026-11-25), docs.webkit.org, dan spec HTML WHATWG.

Singkatan link: `MG` = `https://github.com/egoist/mygo/blob/v0.10.0`.

## Jawaban singkat

**Feasible as-is untuk isolasi level JavaScript**, dengan iframe `sandbox="allow-scripts"` yang dimuat dari custom scheme milik Sasmita (mis. `artifact://<id>/`) dan header CSP ketat. Bridge MyGo memang dirancang supaya iframe tidak bisa bicara ke Go. Ada dua batas yang tidak bisa diatasi iframe:

1. Iframe jalan di WebContent process **yang sama** dengan UI chat (site isolation WebKit masih off by default), jadi tidak ada isolasi terhadap exploit renderer, dan `while(true){}` di artifact membekukan UI chat.
2. Semua custom scheme dianggap trusted oleh MyGo kalau dimuat di main frame. Navigasi top-level ke `artifact://` harus diblokir app.

Kalau threat model Sasmita mencakup exploit renderer atau proteksi hang, perlu **WebView terpisah**. MyGo menyediakannya as-is lewat `Window` terpisah atau `NewWebView`, tetapi `NewWebView` hanya untuk window native UI, bukan window Svelte. Menaruh WKWebView kedua di dalam window Svelte butuh patch MyGo yang tidak kecil.

## Temuan MyGo

### Cara frontend disajikan
- Build: custom scheme `mygo://localhost/` dari file `frontendDist` yang di-embed, lewat `WKURLSchemeHandler`. Bukan localhost HTTP. Dev: dev server `devUrl` (mis. `http://localhost:5173`). ([MG/frontend.go#L11-L21](https://github.com/egoist/mygo/blob/v0.10.0/frontend.go#L11-L21), [MG/docs/frontend.md](https://github.com/egoist/mygo/blob/v0.10.0/docs/frontend.md#how-pages-load))
- Origin frontend di macOS: `mygo://localhost`, secure context.
- `Protocol.Handle(scheme, http.Handler)` mendaftarkan scheme lain. Semua scheme didaftarkan ke `WKWebViewConfiguration` saat window dibuat, jadi harus didaftarkan sebelum window. ([MG/protocol.go#L53-L65](https://github.com/egoist/mygo/blob/v0.10.0/protocol.go#L53-L65), [MG/internal/darwin/window.go#L499-L512](https://github.com/egoist/mygo/blob/v0.10.0/internal/darwin/window.go#L499-L512))

### Injeksi bridge dan scope message handler
- Bridge (`window.mygo`) adalah `WKUserScript` dengan `forMainFrameOnly: true`: `pageScripts()` tidak men-set `AllFrames` untuk bridge, dan backend darwin meneruskan `!s.AllFrames` sebagai `forMainFrameOnly`. Jadi iframe tidak punya `window.mygo`. Hanya `PreloadScript` + `PreloadAllFrames` (opsi app) yang bisa masuk iframe. ([MG/bridge.go#L72-L80](https://github.com/egoist/mygo/blob/v0.10.0/bridge.go#L72-L80), [MG/internal/darwin/window.go#L1727-L1741](https://github.com/egoist/mygo/blob/v0.10.0/internal/darwin/window.go#L1727-L1741), [MG/docs/windows.md#L830](https://github.com/egoist/mygo/blob/v0.10.0/docs/windows.md?plain=1#L830))
- Message handler `mygo` didaftarkan dengan `addScriptMessageHandler:name:` (page world). Menurut header WebKit, ini memasang `window.webkit.messageHandlers.<name>` di **semua frame**. Jadi iframe artifact bisa memanggil `window.webkit.messageHandlers.mygo.postMessage(...)`. ([MG/internal/darwin/window.go#L539-L545](https://github.com/egoist/mygo/blob/v0.10.0/internal/darwin/window.go#L539-L545), [WKUserContentController.h](https://github.com/WebKit/WebKit/blob/main/Source/WebKit/UIProcess/API/Cocoa/WKUserContentController.h))
- MyGo menutup jalur ini dengan dua lapis:
  1. macOS: delegate membuang pesan yang `WKScriptMessage.frameInfo.isMainFrame` = false. ([MG/internal/darwin/window.go#L2545-L2558](https://github.com/egoist/mygo/blob/v0.10.0/internal/darwin/window.go#L2545-L2558))
  2. Setiap window punya secret acak (`crypto/rand`) yang hanya ada di closure bridge. `handleMessage` membuang pesan tanpa prefix secret. ([MG/bridge.go#L278-L290](https://github.com/egoist/mygo/blob/v0.10.0/bridge.go#L278-L290), [MG/docs/architecture.md#L1461-L1467](https://github.com/egoist/mygo/blob/v0.10.0/docs/architecture.md?plain=1#L1461-L1467))
- Trust dihitung dari URL **main frame** (`isTrusted`), bukan dari origin frame pengirim. Ini aman karena pesan subframe sudah dibuang sebelumnya. ([MG/window.go#L1628-L1656](https://github.com/egoist/mygo/blob/v0.10.0/window.go#L1628-L1656))
- MyGo tidak memakai `WKContentWorld` terpisah untuk bridge. `defaultClientWorld` hanya dipakai untuk satu eval internal, dan `Page.Eval` jalan di `pageWorld`. ([MG/internal/darwin/window.go#L1783-L1784](https://github.com/egoist/mygo/blob/v0.10.0/internal/darwin/window.go), [#L2076-L2077](https://github.com/egoist/mygo/blob/v0.10.0/internal/darwin/window.go))

### Trust dan `PageOptions.Untrusted`
- Trusted secara default: `mygo:`, **semua scheme yang di-`Protocol.Handle`**, `file:`, `about:`, origin di `TrustedOrigins`, dan localhost di dev build. ([MG/window.go#L1628-L1656](https://github.com/egoist/mygo/blob/v0.10.0/window.go#L1628-L1656))
- Docs memperingatkan untuk tidak pernah menyajikan HTML pihak ketiga dari custom scheme, atau `LoadHTML` tanpa base URL, di page yang trusted. ([MG/docs/frontend.md#trust](https://github.com/egoist/mygo/blob/v0.10.0/docs/frontend.md#trust))
- `PageOptions.Untrusted` berlaku per window/web view (ditetapkan saat dibuat, bukan per iframe):
  - semua call Go dan `window.mygo.window` ditolak di main thread;
  - pesan di atas 4 KiB dibuang sebelum di-decode;
  - tidak menerima `Broadcast`/`Emit` (hanya `EmitPage`);
  - tanpa izin, window baru, download, atau navigasi ke origin lain, kecuali handler app mengizinkan.
  ([MG/page.go#L375-L392](https://github.com/egoist/mygo/blob/v0.10.0/page.go#L375-L392), [MG/ipc.go#L427-L428](https://github.com/egoist/mygo/blob/v0.10.0/ipc.go#L427-L428), [MG/docs/frontend.md#untrusted-pages](https://github.com/egoist/mygo/blob/v0.10.0/docs/frontend.md#untrusted-pages))
- Untrusted page tetap bisa request ke custom scheme. Handler mengenali page pengirim lewat `mygo.CallerPage(r.Context())`. Itu kanal resmi page untrusted ke app.
- `Untrusted` **tidak berlaku untuk iframe** di dalam window Svelte. Untuk iframe, perlindungannya adalah aturan main-frame-only di atas.

### CSP header dan custom scheme handler
- Tidak ada API CSP khusus. Tetapi handler `Protocol.Handle` adalah `http.Handler` biasa, dan header-nya diteruskan sebagai `NSHTTPURLResponse` `headerFields`. Jadi `Content-Security-Policy` bisa di-set per response. ([MG/internal/darwin/scheme.go#L118-L139](https://github.com/egoist/mygo/blob/v0.10.0/internal/darwin/scheme.go#L118-L139))
- CSP frontend sendiri: tangani scheme `mygo` dengan `mygo.FileServer` + middleware header. Docs menyebut menangani `mygo` menggantikan frontend. Alternatifnya `<meta http-equiv>`. ([MG/docs/frontend.md#custom-protocols](https://github.com/egoist/mygo/blob/v0.10.0/docs/frontend.md#custom-protocols))
- CORS: `allowCORS` tidak memberi header CORS untuk `Origin: null` (opaque origin). Iframe sandbox tidak bisa **membaca** response `mygo://` atau scheme lain. Tetapi request tetap **sampai ke handler**, dan `CallerPage` berisi page main frame yang trusted, karena MyGo tidak tahu frame mana yang meminta. ([MG/protocol.go#L131-L198](https://github.com/egoist/mygo/blob/v0.10.0/protocol.go#L131-L198))
- `Page.Eval` mengabaikan CSP. Itu jalur dari app, bukan dari artifact.
- `ContentRules` (WKContentRuleList) bisa jadi lapis kedua untuk memblokir network. Belum diuji untuk subframe. ([MG/docs/windows.md#blocking-content](https://github.com/egoist/mygo/blob/v0.10.0/docs/windows.md))

### Window atau WebView terpisah
- `Window.NewWebView(WebViewOptions{Page: PageOptions{Untrusted: true}})` membuat WKWebView terpisah dengan secret, content controller, dan opsi `Session` (data store) sendiri. **Hanya untuk window yang menampilkan native UI** (`WindowOptions.Content`). Window yang menampilkan page mendapat `errNoContent`. ([MG/webview.go#L56-L76](https://github.com/egoist/mygo/blob/v0.10.0/webview.go#L56-L76), [MG/docs/ui/web-view.md](https://github.com/egoist/mygo/blob/v0.10.0/docs/ui/web-view.md))
- TAD memakai window Svelte (page), jadi opsi as-is untuk WebView terpisah adalah:
  - (a) `NewWindow` terpisah atau child (`Parent`), dengan `Page.Untrusted` dan URL `artifact://<id>/`;
  - (b) mengganti shell utama ke native UI yang memuat dua `ui.WebView` (frontend trusted + artifact untrusted). Cocok untuk side panel, sulit untuk artifact inline di scroll chat.
- Overlay WKWebView kedua di atas window Svelte (posisi disetir frontend) belum ada dan butuh patch backend.

## Temuan WebKit

- **Site isolation off by default.** `SiteIsolationEnabled` berstatus `unstable`, `defaultValue: false` di WebKit `main` per 2026-11-25. ([UnifiedWebPreferences.yaml](https://github.com/WebKit/WebKit/blob/main/Source/WTF/Scripts/Preferences/UnifiedWebPreferences.yaml), file commit `8084340`) Docs WebKit: tanpa site isolation, iframe cross-site dimuat di process yang sama. Per Januari 2025 proyek ini masih di tahap "fix functionality". ([docs.webkit.org Site Isolation](https://docs.webkit.org/Deep%20Dive/SiteIsolation.html)) Ini mengoreksi `artifacts-genui.md`, yang mengasumsikan iframe cross-site bisa dapat process sendiri.
  - Konsekuensi: iframe artifact berbagi WebContent process dan main thread dengan UI Svelte. Loop tak berujung atau alokasi besar di artifact memengaruhi UI chat, dan exploit renderer bisa menjangkau page trusted.
- **Message handler terlihat di semua frame** dalam content world tempat ia didaftarkan. `WKUserScript.forMainFrameOnly` mengatur injeksi script per frame. `WKFrameInfo.isMainFrame` dan `securityOrigin` tersedia di sisi UI process. ([WKUserContentController.h](https://github.com/WebKit/WebKit/blob/main/Source/WebKit/UIProcess/API/Cocoa/WKUserContentController.h), [WKUserScript.h](https://github.com/WebKit/WebKit/blob/main/Source/WebKit/UIProcess/API/Cocoa/WKUserScript.h), [WKFrameInfo.h](https://github.com/WebKit/WebKit/blob/main/Source/WebKit/UIProcess/API/Cocoa/WKFrameInfo.h))
- **`srcdoc` mewarisi CSP parent.** Menurut spec HTML, policy container `about:srcdoc` adalah clone milik parent. CSP frontend yang ketat (tanpa `'unsafe-inline'`) akan memblokir script inline di srcdoc, dan melonggarkannya melemahkan UI trusted. Dokumen dari custom scheme memakai CSP dari response-nya sendiri. ([HTML: determine navigation params policy container](https://html.spec.whatwg.org/multipage/browsers.html#determining-navigation-params-policy-container)) Karena itu, untuk Tier 2 custom scheme lebih cocok daripada srcdoc.
- **Worker di opaque origin.** Spec: `new Worker` menerima URL same-origin (termasuk `blob:`), dan `data:` URL membuat worker dengan opaque origin. ([HTML Workers](https://html.spec.whatwg.org/multipage/workers.html#dom-worker)) Perilaku WebKit untuk `blob:` dari frame sandbox belum terverifikasi. Worker dedicated tetap di process yang sama, jadi bisa memindahkan komputasi dari main thread tetapi tidak menambah isolasi.
- **Memori per iframe:** tidak ada angka dari sumber primer. Harus diukur.

## Rancangan yang feasible as-is (iframe)

1. Daftarkan `Protocol.Handle("artifact", h)`. `h` hanya menyajikan artifact read-only per id, dengan header:
   - `Content-Security-Policy: sandbox allow-scripts; default-src 'none'; script-src 'unsafe-inline' artifact:; style-src 'unsafe-inline' artifact:; img-src artifact: data: blob:; connect-src 'none'; frame-src 'none'; form-action 'none'`
   - Header `sandbox` menjaga opaque origin walau atribut iframe hilang.
2. Iframe: `<iframe sandbox="allow-scripts" src="artifact://<id>/">`. Tanpa `allow-same-origin`, `allow-top-navigation*`, `allow-popups`, atau `allow-forms`.
3. Bridge: `postMessage` dari iframe ke parent. Parent memeriksa `event.source === iframe.contentWindow` dan `event.origin === "null"`, memakai skema pesan whitelist, dan tidak pernah meneruskan pesan mentah ke Go.
4. Main window: `OnWillNavigate` menolak semua navigasi main frame selain `mygo://localhost`. Ini wajib karena page `artifact://` di main frame akan trusted penuh.
5. Semua handler custom scheme yang punya efek samping menolak `Origin: null`, request tanpa Origin, dan non-POST. Lebih baik: Sasmita tidak punya scheme dengan efek samping, dan semua aksi lewat IPC.
6. Jangan aktifkan `PreloadAllFrames`.
7. Mount lazy, destroy saat off-screen, batasi jumlah iframe hidup.

## Patch kecil yang layak diusulkan ke MyGo (opsional)

- Opsi untuk menandai scheme sebagai untrusted, mis. `Protocol.HandleUntrusted` atau pengecualian di `isTrusted`. Dengan itu, navigasi main frame ke `artifact://` tidak otomatis trusted, dan langkah 4 di atas menjadi pertahanan berlapis, bukan satu-satunya pertahanan.
- Meneruskan info frame atau `Origin` ke `SchemeRequest` supaya `CallerPage` bisa membedakan request dari iframe.

## Kesimpulan

- **Feasible as-is** untuk isolasi JS dari IPC Go, storage, origin, dan navigasi trusted, dengan rancangan di atas.
- **Feasible dengan patch kecil** untuk menutup risiko trust custom scheme di main frame secara struktural.
- **Butuh WebView terpisah** kalau Tier 2 harus tahan exploit renderer atau hang/memori berlebih tanpa membekukan UI chat. Opsi as-is: window terpisah atau shell native UI. Overlay di window Svelte butuh patch besar.

Rekomendasi: mulai dengan iframe di custom scheme untuk artifact inline. Siapkan jalur window/WebView `Untrusted` terpisah untuk artifact berat atau full-screen. Putuskan setelah spike.

## Yang harus diukur spike (macOS 26, Apple Silicon, MyGo v0.10.0)

1. Dari iframe `artifact://` sandbox: `window.mygo` undefined, `window.webkit.messageHandlers.mygo` ada, `postMessage` ke sana tidak sampai ke `handleMessage` (log Go), dan tidak ada call bound yang jalan.
2. CSP header custom scheme dihormati WKWebView, termasuk directive `sandbox` (cek `origin === "null"`), dan `connect-src 'none'` memblokir fetch, WebSocket, beacon `<img>`, serta `fetch("mygo://localhost/")`.
3. Bandingkan srcdoc: konfirmasi pewarisan CSP parent di WebKit.
4. Header `Origin` dan `CallerPage` yang dilihat handler untuk GET, POST, dan `<img>` dari iframe.
5. Navigasi keluar dari iframe (`top.location`, `<a target=_top>`, `window.open`, form) gagal. `OnWillNavigate` main window memblokir `artifact://` di main frame.
6. Worker dari `blob:`, `data:`, dan module worker di frame opaque. Perilaku `localStorage`/IndexedDB/cookie (diharapkan SecurityError).
7. Jumlah WebContent process dan memori (`footprint`/`vmmap --summary` per PID, plus total app) untuk 0, 1, 5, dan 20 iframe artifact, lalu setelah iframe di-destroy.
8. `while(true){}` dan alokasi 1 GB di iframe: apakah UI chat beku atau process crash? Bisakah app pulih (reload iframe, `webViewWebContentProcessDidTerminate`)?
9. Window terpisah `Untrusted`: apakah dapat WebContent process sendiri, berapa overhead memori dan waktu buka, dan apakah hang di sana tidak memengaruhi window utama.
10. Latensi dan ukuran pesan `postMessage` parent ⇄ iframe untuk update artifact streaming.
11. Opsional: efek `ContentRules` pada request subframe.
