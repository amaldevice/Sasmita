package artifacts

import (
	"encoding/json"
	"fmt"
)

// RuntimeImports is the complete, version-pinned import surface available to a
// sandboxed artifact. Anything outside this map fails before the artifact can
// reach a CDN (there is no network in the iframe anyway: connect-src 'none').
var RuntimeImports = map[string]string{
	"react":                                  "artifact://runtime/1/react.js",
	"react-dom/client":                       "artifact://runtime/1/react-dom-client.js",
	"d3":                                     "artifact://runtime/1/d3.js",
	"chart.js":                               "artifact://runtime/1/chart.js",
	"lucide":                                 "artifact://runtime/1/lucide.js",
	"tailwindcss":                            "artifact://runtime/1/tailwind.js",
	"three":                                  "artifact://runtime/1/three.js",
	"three/addons/controls/OrbitControls.js": "artifact://runtime/1/three-orbit-controls.js",
	"artifact-runtime/bridge":                "artifact://runtime/1/bridge.js",
}

// RuntimeModule returns the local ESM source for one pinned runtime module.
func RuntimeModule(path string) (string, bool) {
	src, ok := runtimeModules[path]
	return src, ok
}

// ImportMapJSON renders the import map injected into every served document.
func ImportMapJSON() (string, error) {
	b, err := json.Marshal(struct {
		Imports map[string]string `json:"imports"`
	}{Imports: RuntimeImports})
	return string(b), err
}

// KnownRuntimePaths lists every locally served runtime module path.
func KnownRuntimePaths() []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(RuntimeImports))
	for _, target := range RuntimeImports {
		path := target[len("artifact://runtime/1/"):]
		if !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}
	return out
}

func runtimeStub(name, exports string) string {
	return fmt.Sprintf(`// %s/%s — locally served, version-pinned module.
// Full third-party bundles are installed at packaging time (S1 packaged check).
const RUNTIME = %q;
export const runtimeVersion = RUNTIME;
export const bundled = false;
%s
export default { runtimeVersion, bundled };
`, RuntimeVersion, name, RuntimeVersion, exports)
}

var runtimeModules = map[string]string{
	"react.js": runtimeStub("react", `
function unavailable(what) {
  throw new Error(RUNTIME + ": React bundle is not installed in this build (" + what + ")");
}
export function createElement(type, props, ...children) { return unavailable("createElement"); }
export const Fragment = Symbol("react.fragment");
export function useState(initial) { return unavailable("useState"); }
export function useEffect(fn, deps) { return unavailable("useEffect"); }
export function useMemo(fn, deps) { return unavailable("useMemo"); }
export function useRef(initial) { return unavailable("useRef"); }
export function useCallback(fn, deps) { return unavailable("useCallback"); }
`),
	"react-dom-client.js": runtimeStub("react-dom/client", `
function unavailable(what) {
  throw new Error(RUNTIME + ": ReactDOM bundle is not installed in this build (" + what + ")");
}
export function createRoot(container) { return unavailable("createRoot"); }
export function hydrateRoot(container, element) { return unavailable("hydrateRoot"); }
`),
	"d3.js": runtimeStub("d3", `
function unavailable(what) {
  throw new Error(RUNTIME + ": d3 bundle is not installed in this build (" + what + ")");
}
export const select = (...a) => unavailable("select");
export const scaleLinear = (...a) => unavailable("scaleLinear");
export const scaleBand = (...a) => unavailable("scaleBand");
`),
	"chart.js": runtimeStub("chart.js", `
export class Chart {
  constructor(...args) {
    throw new Error(RUNTIME + ": Chart.js bundle is not installed in this build");
  }
}
export default Chart;
export function register(...items) { return items; }
`),
	"lucide.js": runtimeStub("lucide", `
export function createIcons(...args) {
  throw new Error(RUNTIME + ": lucide bundle is not installed in this build");
}
`),
	"tailwind.js": runtimeStub("tailwind browser runtime", `
export function tailwind(...args) {
  throw new Error(RUNTIME + ": Tailwind browser runtime is not installed in this build");
}
`),
	"three.js": runtimeStub("three core", `
export class Scene { constructor() { throw new Error(RUNTIME + ": three.js bundle is not installed in this build"); } }
export class WebGLRenderer { constructor() { throw new Error(RUNTIME + ": three.js bundle is not installed in this build"); } }
`),
	"three-orbit-controls.js": runtimeStub("three OrbitControls", `
export class OrbitControls {
  constructor() { throw new Error(RUNTIME + ": three.js OrbitControls bundle is not installed in this build"); }
}
`),
	"bridge.js": `// artifact-runtime/bridge — the only channel from an artifact to its host.
// All messages are postMessage'd to the parent and whitelisted there.
const RUNTIME = "` + RuntimeVersion + `";
export const runtimeVersion = RUNTIME;

function send(type, payload) {
  parent.postMessage({ channel: "sasmita.artifact", version: 1, type, payload }, "*");
}

export function resize(width, height) { send("resize", { width, height }); }
export function reportError(message) { send("reportError", { message: String(message) }); }
export function saveState(state) { send("saveState", { state }); }
export function loadState() { return new Promise((resolve) => {
  const onMessage = (event) => {
    if (event.source !== parent) return;
    const msg = event.data;
    if (!msg || msg.channel !== "sasmita.artifact" || msg.type !== "state") return;
    window.removeEventListener("message", onMessage);
    resolve(msg.state);
  };
  window.addEventListener("message", onMessage);
  send("loadState", {});
}); }
export function getData(ref) { return new Promise((resolve) => {
  const onMessage = (event) => {
    if (event.source !== parent) return;
    const msg = event.data;
    if (!msg || msg.channel !== "sasmita.artifact" || msg.type !== "data") return;
    window.removeEventListener("message", onMessage);
    resolve(msg.data);
  };
  window.addEventListener("message", onMessage);
  send("getData", { ref });
}); }
export function suggestPrompt(prompt) { send("suggestPrompt", { prompt: String(prompt) }); }
`,
}
