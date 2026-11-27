package onboarding

// ExamplePrompt is declarative data. VisualHint records the intended visual
// (flow, chart, comparison, timeline) for later UI-block/artifact tickets;
// nothing here claims a visual is rendered today.
type ExamplePrompt struct {
	ID         string `json:"id"`
	Prompt     string `json:"prompt"`
	TitleID    string `json:"titleId"`
	TitleEN    string `json:"titleEn"`
	DetailID   string `json:"detailId"`
	DetailEN   string `json:"detailEn"`
	VisualHint string `json:"visualHint"`
}

var examplePrompts = []ExamplePrompt{
	{
		ID:         "training-flow",
		Prompt:     "Jelaskan alur training neural network dengan diagram",
		TitleID:    "Alur training neural network",
		TitleEN:    "Neural-network training flow",
		DetailID:   "Lihat langkahnya sebagai alur",
		DetailEN:   "See the steps as a flow",
		VisualHint: "flow",
	},
	{
		ID:         "sales-trend",
		Prompt:     "Tunjukkan tren penjualan per bulan dari workbook ini",
		TitleID:    "Tren penjualan bulanan",
		TitleEN:    "Monthly sales trend",
		DetailID:   "Ubah angka menjadi grafik",
		DetailEN:   "Turn the numbers into a chart",
		VisualHint: "chart",
	},
	{
		ID:         "research-compare",
		Prompt:     "Bandingkan dua pendekatan riset ini dalam tabel",
		TitleID:    "Bandingkan dua pendekatan",
		TitleEN:    "Compare two approaches",
		DetailID:   "Perbedaan yang mudah dibaca",
		DetailEN:   "Differences at a glance",
		VisualHint: "comparison",
	},
	{
		ID:         "decision-timeline",
		Prompt:     "Ubah catatan rapat saya menjadi linimasa keputusan",
		TitleID:    "Linimasa keputusan rapat",
		TitleEN:    "Meeting decision timeline",
		DetailID:   "Urutan keputusan dari catatan",
		DetailEN:   "Decisions in order from your notes",
		VisualHint: "timeline",
	},
}

// ListExamplePrompts returns a copy so callers cannot mutate the catalog.
func ListExamplePrompts() []ExamplePrompt {
	return append([]ExamplePrompt(nil), examplePrompts...)
}
