package usage

import "strconv"

// rate is USD per one million tokens. These are deliberately conservative,
// application-side figures used only when a provider did not report a cost.
// A cost built from them is always marked CostEstimated in the UI; it is never
// presented as a provider measurement (TAD §13.3, §15.3).
type rate struct {
	input, output, cacheRead, cacheWrite float64
}

var defaultRate = rate{input: 0.15, output: 0.60, cacheRead: 0.0375, cacheWrite: 0.1875}

var modelRates = map[string]rate{
	"gpt-4o":            {input: 2.50, output: 10.00, cacheRead: 1.25, cacheWrite: 3.125},
	"gpt-4o-mini":       defaultRate,
	"claude-3-5-sonnet": {input: 3.00, output: 15.00, cacheRead: 0.30, cacheWrite: 3.75},
	"claude-3-5-haiku":  {input: 0.80, output: 4.00, cacheRead: 0.08, cacheWrite: 1.00},
	"claude-3-7-sonnet": {input: 3.00, output: 15.00, cacheRead: 0.30, cacheWrite: 3.75},
}

func parseFloatCost(raw string) (float64, error) {
	if raw == "" {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseFloat(raw, 64)
}

// EstimatedCostUSD derives a rough cost from token counts. Input tokens in
// usage_records include cached tokens (ADR-0001 / #17 normalization), so only
// the uncached portion is charged at the full input rate.
func EstimatedCostUSD(r Record) float64 {
	rate := defaultRate
	if known, ok := modelRates[r.ModelID]; ok {
		rate = known
	}
	uncached := r.UncachedTokens
	if uncached == 0 {
		uncached = r.InputTokens - r.CacheReadTokens - r.CacheWriteTokens
	}
	if uncached < 0 {
		uncached = 0
	}
	return (float64(uncached)*rate.input +
		float64(r.CacheReadTokens)*rate.cacheRead +
		float64(r.CacheWriteTokens)*rate.cacheWrite +
		float64(r.OutputTokens)*rate.output) / 1_000_000
}
