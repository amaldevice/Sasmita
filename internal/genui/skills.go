package genui

// skillDocs are the only per-component documentation the agent loads. Keep them
// compact: render_ui stays generic and these are fetched only when relevant.
var skillDocs = map[string]string{
	"chart": `chart: put small series in "values" as objects {label, value, series?}; ` +
		`for large series use "dataRef" to a declared data key with an artifact:// source. ` +
		`Label every series directly on the plot; never use a colour-box legend. ` +
		`Max 5000 inline points per series. Include units in the label.`,
	"table": `table: small grids use "values" as an array of row objects; large grids use ` +
		`"dataRef" and the host's bounded fetch. Column headers are the labels. ` +
		`Max 200 inline rows; paginate or use an artifact beyond that. ` +
		`No colour-box legend; keep numbers in monospace.`,
	"mermaid": `mermaid: "source" holds validated diagram text (for example A[Start] --> B[End]) ` +
		`or use "dataRef". Nodes carry direct text labels. No arbitrary HTML or scripts. ` +
		`Keep it small; load a heavy diagram renderer only when needed.`,
	"kpi": `kpi: "values" is an array of {label, value} metrics, or use "dataRef". ` +
		`Render label and value at the same typographic weight in one sentence-like row. ` +
		`Do not use a big number above a small label. State the unit with the value.`,
	"math": `math: "source" is the expression (for example E = mc^2) and "label" names it. ` +
		`Explain variables in the block summary. No eval, no script tags, no external fonts.`,
	"slider": `slider: requires "bind": "state.<key>" where the key is declared in "state", ` +
		`plus "min" < "max" and an optional positive "step". Changing it is view-local: ` +
		`update immediately in Svelte, then persist a debounced state patch. Never call the model.`,
	"tabs": `tabs: requires "bind" to a declared state key and 2-20 "options". ` +
		`Selecting an option is a view-local change; do not call the model. ` +
		`Use plain text labels, not pills or colour boxes.`,
	"toggle": `toggle: requires "bind" to a declared boolean state key. ` +
		`It flips a view-local flag with no model call; persist the committed value debounced. ` +
		`Do not silently turn a toggle into a tool action.`,
}
