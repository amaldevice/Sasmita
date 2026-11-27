package artifacts

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// CSP is applied to every artifact:// response by the Go scheme handler.
// The sandbox directive keeps the document at an opaque origin even if the
// iframe attribute is ever lost; connect-src 'none' makes network impossible.
const CSP = "sandbox allow-scripts; default-src 'none'; " +
	"script-src 'unsafe-inline' artifact:; style-src 'unsafe-inline' artifact:; " +
	"img-src artifact: data: blob:; connect-src 'none'; frame-src 'none'; " +
	"form-action 'none'; base-uri 'none'"

// Handler serves artifact:// requests. It is strictly read-only: every branch
// reads stored bytes and writes only an HTTP response. All state changes go
// through the Service and the host-side postMessage bridge.
type Handler struct {
	Store *Store
}

func NewHandler(store *Store) http.Handler { return &Handler{Store: store} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	headers(w)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "artifact:// is read-only", http.StatusMethodNotAllowed)
		return
	}
	if h.Store == nil {
		http.Error(w, "artifact store is not configured", http.StatusServiceUnavailable)
		return
	}

	host := r.URL.Host
	if host == "runtime" {
		h.serveRuntime(w, r.URL.Path)
		return
	}
	if !validID(host) {
		http.Error(w, "invalid artifact id", http.StatusBadRequest)
		return
	}
	a, err := h.Store.Load(host)
	if err != nil {
		if err == ErrNotFound {
			http.Error(w, "artifact not found", http.StatusNotFound)
		} else {
			http.Error(w, "artifact unavailable", http.StatusInternalServerError)
		}
		return
	}
	v, err := h.selectVersion(a, r.URL.Query().Get("v"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch r.URL.Path {
	case "/", "/index.html":
		h.serveDocument(w, a, v)
	case "/script.js":
		if v.Kind != SourceJSX {
			http.Error(w, "this artifact is an HTML document, not a JS module", http.StatusNotFound)
			return
		}
		serve(w, v.MIMEType, v.Compiled)
	default:
		http.Error(w, "artifact path not found", http.StatusNotFound)
	}
}

func headers(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Security-Policy", CSP)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
}

func serve(w http.ResponseWriter, mime, body string) {
	w.Header().Set("Content-Type", mime)
	_, _ = w.Write([]byte(body))
}

func (h *Handler) serveRuntime(w http.ResponseWriter, path string) {
	if !strings.HasPrefix(path, "/1/") {
		http.Error(w, "unknown artifact-runtime version; expected /1/<module>", http.StatusNotFound)
		return
	}
	name := strings.TrimPrefix(path, "/1/")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		http.Error(w, "unknown artifact-runtime/1 module", http.StatusNotFound)
		return
	}
	src, ok := RuntimeModule(name)
	if !ok {
		http.Error(w, fmt.Sprintf("unknown artifact-runtime/1 import %q; no CDN fallback is allowed", name), http.StatusNotFound)
		return
	}
	serve(w, "text/javascript; charset=utf-8", src)
}

func (h *Handler) selectVersion(a Artifact, raw string) (Version, error) {
	if raw == "" {
		return a.Latest()
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return Version{}, fmt.Errorf("invalid artifact version %q", raw)
	}
	for _, v := range a.Versions {
		if v.Number == n {
			return v, nil
		}
	}
	return Version{}, fmt.Errorf("artifact %s has no version %d", a.ID, n)
}

func (h *Handler) serveDocument(w http.ResponseWriter, a Artifact, v Version) {
	importMap, err := ImportMapJSON()
	if err != nil {
		http.Error(w, "import map unavailable", http.StatusInternalServerError)
		return
	}
	head := `<meta charset="utf-8">
<script type="importmap">` + importMap + `</script>`

	// A froze_view artifact is never auto-run: the host marks it and shows a
	// neutral card instead. The handler still serves metadata without scripts.
	if a.Status == StatusFrozeView {
		doc := "<!doctype html><html><head>" + head + "</head><body>" +
			"<p>Artifact froze the view and will not run automatically.</p></body></html>"
		serve(w, "text/html; charset=utf-8", doc)
		return
	}

	if v.Kind == SourceJSX {
		doc := "<!doctype html><html><head>" + head + "</head><body><div id=\"root\"></div>" +
			"<script type=\"module\" src=\"./script.js\"></script></body></html>"
		serve(w, "text/html; charset=utf-8", doc)
		return
	}
	serve(w, "text/html; charset=utf-8", injectHead(v.Compiled, head))
}

// injectHead adds the import map to a stored HTML document without rewriting
// any of its own markup.
func injectHead(doc, head string) string {
	lower := strings.ToLower(doc)
	if i := strings.Index(lower, "<head>"); i >= 0 {
		at := i + len("<head>")
		return doc[:at] + "\n" + head + "\n" + doc[at:]
	}
	if i := strings.Index(lower, "<html"); i >= 0 {
		if end := strings.Index(doc[i:], ">"); end >= 0 {
			at := i + end + 1
			return doc[:at] + "\n<head>" + head + "</head>\n" + doc[at:]
		}
	}
	return "<!doctype html><html><head>" + head + "</head><body>" + doc + "</body></html>"
}
