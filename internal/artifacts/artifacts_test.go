package artifacts_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/artifacts"
)

func newService(t *testing.T) (*artifacts.Service, *artifacts.Store) {
	t.Helper()
	store := artifacts.NewStore(t.TempDir())
	return &artifacts.Service{Store: store}, store
}

func createHTML(t *testing.T, svc *artifacts.Service, source string) artifacts.Artifact {
	t.Helper()
	a, err := svc.Create(context.Background(), artifacts.CreateInput{Source: source, SourceKind: artifacts.SourceHTML})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestSeam1CreateThenEditAreImmutableVersions(t *testing.T) {
	svc, store := newService(t)
	source := "<h1>Hello</h1><p>one apple</p>"
	a := createHTML(t, svc, source)
	if a.Ref != "artifact://"+a.ID || a.RuntimeVersion != artifacts.RuntimeVersion {
		t.Fatalf("artifact = %+v", a)
	}
	if len(a.Versions) != 1 || a.Versions[0].Number != 1 || a.Versions[0].Source != source {
		t.Fatalf("versions = %+v", a.Versions)
	}
	firstSHA := a.Versions[0].SHA256

	edited, err := svc.Edit(context.Background(), a.ID, []artifacts.Edit{{Old: "one apple", New: "two apples"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(edited.Versions) != 2 {
		t.Fatalf("versions = %d, want 2", len(edited.Versions))
	}
	if edited.Versions[0].SHA256 != firstSHA || edited.Versions[0].Source != source {
		t.Fatal("edit mutated version 1")
	}
	if want := "<h1>Hello</h1><p>two apples</p>"; edited.Versions[1].Source != want {
		t.Fatalf("v2 source = %q, want %q", edited.Versions[1].Source, want)
	}

	// Reload from disk: the new version must be durable, the old one intact.
	reloaded, err := store.Load(a.ID)
	if err != nil || len(reloaded.Versions) != 2 || reloaded.Versions[0].Source != source {
		t.Fatalf("reload = %+v (err %v)", reloaded, err)
	}
}

func TestSeam1EditRejectsZeroAndMultipleMatches(t *testing.T) {
	svc, store := newService(t)
	a := createHTML(t, svc, "<p>cat</p><p>cat</p><p>dog</p>")

	for _, tc := range []struct {
		name  string
		edits []artifacts.Edit
		want  string
	}{
		{"zero matches", []artifacts.Edit{{Old: "bird", New: "fish"}}, "not found"},
		{"multiple matches", []artifacts.Edit{{Old: "cat", New: "fish"}}, "matches 2 times"},
		{"empty edits", nil, "no edits"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Edit(context.Background(), a.ID, tc.edits)
			if !artifacts.IsEditError(err) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want EditError containing %q", err, tc.want)
			}
			current, loadErr := store.Load(a.ID)
			if loadErr != nil || len(current.Versions) != 1 {
				t.Fatalf("rejected edit appended a version: %+v (err %v)", current, loadErr)
			}
		})
	}
}

func TestSeam1RewriteAppendsImmutableVersion(t *testing.T) {
	svc, _ := newService(t)
	a := createHTML(t, svc, "<p>old</p>")
	rewritten, err := svc.Rewrite(context.Background(), a.ID, artifacts.RewriteInput{Source: "<p>entirely new</p>"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rewritten.Versions) != 2 || rewritten.Versions[0].Source != "<p>old</p>" || rewritten.Versions[1].Source != "<p>entirely new</p>" {
		t.Fatalf("versions = %+v", rewritten.Versions)
	}
}

func TestSeam1SourceAndDataRefLimits(t *testing.T) {
	svc, _ := newService(t)
	if _, err := svc.Create(context.Background(), artifacts.CreateInput{Source: strings.Repeat("x", artifacts.MaxSourceBytes+1)}); !errors.Is(err, artifacts.ErrSourceTooBig) {
		t.Fatalf("oversized source err = %v", err)
	}

	tooBig := artifacts.Manifest{DataRefs: map[string]artifacts.DataRefSpec{
		"points": {Ref: "artifact://data/points", SizeBytes: artifacts.MaxDataRefBytes + 1},
	}}
	if _, err := svc.Create(context.Background(), artifacts.CreateInput{Source: "<p>hi</p>", Manifest: tooBig}); !errors.Is(err, artifacts.ErrDataRefTooBig) {
		t.Fatalf("oversized data ref err = %v", err)
	}

	// The host-side resolver catches a ref that lies about its size.
	svc.Data = sizeResolver{size: artifacts.MaxDataRefBytes + 1}
	declared := artifacts.Manifest{DataRefs: map[string]artifacts.DataRefSpec{
		"points": {Ref: "artifact://data/points", SizeBytes: 10},
	}}
	if _, err := svc.Create(context.Background(), artifacts.CreateInput{Source: "<p>hi</p>", Manifest: declared}); !errors.Is(err, artifacts.ErrDataRefTooBig) {
		t.Fatalf("resolver did not enforce data cap: %v", err)
	}
}

type sizeResolver struct{ size int }

func (r sizeResolver) Size(string) (int, error) { return r.size, nil }

func TestSeam1JSXIsTranspiledByEsbuild(t *testing.T) {
	svc, store := newService(t)
	a, err := svc.Create(context.Background(), artifacts.CreateInput{
		SourceKind: artifacts.SourceJSX,
		Source:     `export default function App(){ return <div className="box">hi</div>; }`,
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := store.VersionSource(a.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(v.Compiled, "<div") || !strings.Contains(v.Compiled, "createElement") {
		t.Fatalf("JSX was not transpiled: %s", v.Compiled)
	}
	if v.MIMEType != "text/javascript; charset=utf-8" {
		t.Fatalf("mime = %q", v.MIMEType)
	}
}

func TestSeam1HandlerCSPReadOnlyAndImportMap(t *testing.T) {
	svc, store := newService(t)
	a, err := svc.Create(context.Background(), artifacts.CreateInput{
		SourceKind: artifacts.SourceJSX,
		Source:     `export default () => <p>hello</p>;`,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := artifacts.NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, a.Ref+"/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, body)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"sandbox allow-scripts", "default-src 'none'", "connect-src 'none'"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP %q missing %q", csp, want)
		}
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff header")
	}
	importMap, err := artifacts.ImportMapJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, importMap) {
		t.Fatalf("document has no artifact-runtime/1 import map: %s", body)
	}
	if strings.Contains(body, "srcdoc") {
		t.Fatal("document uses srcdoc")
	}

	// Every pinned library resolves to a locally served module.
	for _, spec := range []string{"react", "react-dom/client", "d3", "chart.js", "lucide", "tailwindcss", "three", "three/addons/controls/OrbitControls.js"} {
		if !strings.Contains(importMap, spec) {
			t.Fatalf("import map missing %q", spec)
		}
	}
	for _, path := range artifacts.KnownRuntimePaths() {
		r := httptest.NewRequest(http.MethodGet, "artifact://runtime/1/"+path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), artifacts.RuntimeVersion) {
			t.Fatalf("runtime module %s = %d: %s", path, w.Code, w.Body.String())
		}
	}

	unknown := httptest.NewRequest(http.MethodGet, "artifact://runtime/1/evil.js", nil)
	uw := httptest.NewRecorder()
	h.ServeHTTP(uw, unknown)
	if uw.Code != http.StatusNotFound || !strings.Contains(uw.Body.String(), "unknown artifact-runtime/1 import") {
		t.Fatalf("unknown import = %d: %s", uw.Code, uw.Body.String())
	}

	// The handler is read-only even when a subframe sends a POST.
	post := httptest.NewRequest(http.MethodPost, a.Ref+"/", nil)
	pw := httptest.NewRecorder()
	h.ServeHTTP(pw, post)
	if pw.Code != http.StatusMethodNotAllowed || pw.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST = %d, Allow=%q", pw.Code, pw.Header().Get("Allow"))
	}
}

func TestSeam1HandlerServesCompiledScriptAndRespectsFrozeView(t *testing.T) {
	svc, store := newService(t)
	a, err := svc.Create(context.Background(), artifacts.CreateInput{SourceKind: artifacts.SourceJSX, Source: `export default () => <p>x</p>;`})
	if err != nil {
		t.Fatal(err)
	}
	h := artifacts.NewHandler(store)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, a.Ref+"/script.js", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "createElement") {
		t.Fatalf("script.js = %d: %s", w.Code, w.Body.String())
	}

	if err := svc.MarkFroze(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, a.Ref+"/", nil))
	if strings.Contains(w.Body.String(), "createElement") || strings.Contains(w.Body.String(), "script.js") {
		t.Fatal("froze_view artifact would auto-run")
	}
}

func TestSeam1MainFrameNavigationRejected(t *testing.T) {
	if err := artifacts.RejectMainFrameNavigation("artifact://abc/"); !errors.Is(err, artifacts.ErrMainFrameNavigation) {
		t.Fatalf("artifact:// navigation err = %v", err)
	}
	for _, ok := range []string{"mygo://localhost/", "https://example.com", "about:blank"} {
		if err := artifacts.RejectMainFrameNavigation(ok); err != nil {
			t.Fatalf("rejected %q: %v", ok, err)
		}
	}
}

func TestSeam1ArtifactFilesAreAtomic(t *testing.T) {
	svc, store := newService(t)
	a := createHTML(t, svc, "<p>atomic</p>")
	base := filepath.Join(store.Dir, a.ID)
	for _, name := range []string{"meta.json", "v1.source", "v1.compiled"} {
		if _, err := os.Stat(filepath.Join(base, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("leftover temp file %s", e.Name())
		}
	}
}
