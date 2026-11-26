package usage_test

import (
	"math"
	"testing"

	"github.com/amaldevice/sasmita/internal/usage"
)

// Records come from a real reported usage stream; the expected figures below
// are worked out by hand, not recomputed with the aggregation's own formula.
func reportRecords() []usage.Record {
	return []usage.Record{
		{Day: "2026-11-24", ProviderID: "openai", ModelID: "gpt-test",
			InputTokens: 1000, OutputTokens: 100, CacheReadTokens: 800, UncachedTokens: 200},
		{Day: "2026-11-25", ProviderID: "openai", ModelID: "gpt-test",
			InputTokens: 1000, OutputTokens: 110, CacheReadTokens: 750, UncachedTokens: 250},
		{Day: "2026-11-26", ProviderID: "openai", ModelID: "gpt-test",
			InputTokens: 1000, OutputTokens: 120, CacheReadTokens: 400, UncachedTokens: 600},
		{Day: "2026-11-26", ProviderID: "anthropic", ModelID: "claude-test",
			InputTokens: 500, OutputTokens: 50, CacheReadTokens: 100, UncachedTokens: 400,
			CacheWriteTokens: 0, Estimated: true},
	}
}

func rowByKey(rows []usage.Row, key string) (usage.Row, bool) {
	for _, row := range rows {
		if row.Key == key {
			return row, true
		}
	}
	return usage.Row{}, false
}

func TestAggregateGroupsByModelAndDay(t *testing.T) {
	report := usage.Aggregate(reportRecords())

	if len(report.Models) != 2 {
		t.Fatalf("models = %d, want 2: %+v", len(report.Models), report.Models)
	}
	if len(report.Days) != 3 {
		t.Fatalf("days = %d, want 3: %+v", len(report.Days), report.Days)
	}

	model, ok := rowByKey(report.Models, "gpt-test")
	if !ok {
		t.Fatal("missing gpt-test model row")
	}
	// 800 + 750 + 400 = 1950 cache reads over 3000 input = 65 %.
	if model.Requests != 3 || model.InputTokens != 3000 || model.CacheReadTokens != 1950 ||
		math.Abs(model.HitRate-0.65) > 1e-9 {
		t.Fatalf("gpt-test row = %+v, want requests=3 input=3000 cache=1950 hit=0.65", model)
	}

	day, ok := rowByKey(report.Days, "2026-11-26")
	if !ok {
		t.Fatal("missing 2026-11-26 day row")
	}
	// Two requests that day: (400 + 100) cache reads over 1500 input = 1/3.
	if day.Requests != 2 || math.Abs(day.HitRate-1.0/3.0) > 1e-9 {
		t.Fatalf("2026-11-26 row = %+v, want requests=2 hit=1/3", day)
	}
}

func TestAggregateFlagsHitRateDropOnlyInReport(t *testing.T) {
	report := usage.Aggregate(reportRecords())
	if report.Drop == nil {
		t.Fatal("expected a hit-rate drop flag")
	}
	// 75 % on the 25th down to 33.3 % on the 26th is a clear drop.
	if report.Drop.Day != "2026-11-26" || math.Abs(report.Drop.From-0.75) > 1e-9 {
		t.Fatalf("drop = %+v, want day=2026-11-26 from=0.75", *report.Drop)
	}

	steady := usage.Aggregate([]usage.Record{
		{Day: "2026-11-25", InputTokens: 100, CacheReadTokens: 80, UncachedTokens: 20},
		{Day: "2026-11-26", InputTokens: 100, CacheReadTokens: 79, UncachedTokens: 21},
	})
	if steady.Drop != nil {
		t.Fatalf("1 percentage point change must not be flagged: %+v", *steady.Drop)
	}
}

func TestAggregateMarksAppEstimatedCost(t *testing.T) {
	report := usage.Aggregate(reportRecords())
	model, _ := rowByKey(report.Models, "gpt-test")
	if !model.CostEstimated {
		t.Fatal("cost from token counts must be labelled an app estimate")
	}
	if model.CostUSD <= 0 {
		t.Fatalf("cost estimate = %v, want a positive number", model.CostUSD)
	}

	// A provider-reported cost is used verbatim and is not called an estimate.
	reported := usage.Aggregate([]usage.Record{
		{ModelID: "m", InputTokens: 100, OutputTokens: 10, CostEstimate: "0.00042"},
	})
	row, _ := rowByKey(reported.Models, "m")
	if row.CostEstimated || math.Abs(row.CostUSD-0.00042) > 1e-12 {
		t.Fatalf("reported cost row = %+v, want cost=0.00042 estimated=false", row)
	}
}
