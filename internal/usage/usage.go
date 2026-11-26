// Package usage holds read models for the Usage UI. It is a leaf: no SQL, no
// provider SDK, no desktop framework. Aggregation is pure so both the Go
// service and its tests observe the same figures (TAD §13.3, §15.3).
package usage

import "sort"

// HitRateDropThreshold is the smallest fall between consecutive days that is
// worth surfacing in Diagnostics. Small day-to-day noise is not a warning.
const HitRateDropThreshold = 0.10

// Record is one usage_records row with its calendar day joined from the run.
type Record struct {
	Day              string
	ProviderID       string
	ModelID          string
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	UncachedTokens   int64
	// Estimated marks token counts the application inferred, not provider-reported.
	Estimated bool
	// CostEstimate is non-empty only when the provider reported a cost.
	CostEstimate string
}

// Row is one aggregated line in Diagnostics. Key is a model id (Models) or a
// YYYY-MM-DD day (Days). HitRate is a fraction in [0,1]: cache reads over all
// input tokens, where input is the finalized cacheRead + cacheWrite + uncached.
type Row struct {
	Key              string
	ProviderID       string
	Requests         int64
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	UncachedTokens   int64
	HitRate          float64
	CostUSD          float64
	CostEstimated    bool
}

// Drop is a material fall in the aggregate hit rate between two days. It is
// returned only by the Diagnostics report, never by the chat read path.
type Drop struct {
	Day  string
	From float64
	To   float64
}

// Report is the Diagnostics read model: per-model and per-day aggregates plus
// an optional hit-rate warning.
type Report struct {
	Models []Row
	Days   []Row
	Drop   *Drop
}

// Aggregate rolls usage records up by model and by day. Every cost that was
// not provider-reported is recomputed and flagged as an application estimate.
func Aggregate(records []Record) Report {
	models := map[string]*Row{}
	days := map[string]*Row{}
	for _, r := range records {
		model := rowFor(models, r.ModelID)
		day := rowFor(days, r.Day)
		if model.ProviderID == "" {
			model.ProviderID = r.ProviderID
		}
		add(model, r)
		add(day, r)
	}

	report := Report{}
	for _, rows := range []map[string]*Row{models, days} {
		for key, row := range rows {
			row.Key = key
			row.HitRate = HitRate(row.InputTokens, row.CacheReadTokens)
		}
	}
	for _, row := range models {
		report.Models = append(report.Models, *row)
	}
	for _, row := range days {
		report.Days = append(report.Days, *row)
	}
	sort.Slice(report.Models, func(i, j int) bool { return report.Models[i].Key < report.Models[j].Key })
	sort.Slice(report.Days, func(i, j int) bool { return report.Days[i].Key < report.Days[j].Key })

	var previous float64
	for i, row := range report.Days {
		if i > 0 && row.HitRate < previous-HitRateDropThreshold {
			report.Drop = &Drop{Day: row.Key, From: previous, To: row.HitRate}
			break
		}
		previous = row.HitRate
	}
	return report
}

func rowFor(rows map[string]*Row, key string) *Row {
	row := rows[key]
	if row == nil {
		row = &Row{}
		rows[key] = row
	}
	return row
}

func add(row *Row, r Record) {
	row.Requests++
	row.InputTokens += r.InputTokens
	row.OutputTokens += r.OutputTokens
	row.CacheReadTokens += r.CacheReadTokens
	row.CacheWriteTokens += r.CacheWriteTokens
	row.UncachedTokens += r.UncachedTokens

	if reported, err := parseFloatCost(r.CostEstimate); err == nil && reported > 0 {
		row.CostUSD += reported
	} else {
		row.CostUSD += EstimatedCostUSD(r)
		row.CostEstimated = true
	}
}
