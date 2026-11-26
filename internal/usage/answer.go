package usage

// AnswerUsage is the per-answer read model behind the hover popover and the
// optional monospace line. It is derived from the usage record of the run that
// produced the answer, never from a mutable conversation field.
type AnswerUsage struct {
	ProviderID       string
	ModelID          string
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	UncachedTokens   int64
	// TokenEstimated marks counts the application inferred rather than read.
	TokenEstimated bool
	HitRate        float64
	CostUSD        float64
	CostEstimated  bool
	DurationMs     int64
}

// ViewOf computes the figures shown for a single answer. Cost that was not
// reported by the provider is recomputed and marked as an application estimate.
func ViewOf(r Record) AnswerUsage {
	view := AnswerUsage{
		ProviderID:       r.ProviderID,
		ModelID:          r.ModelID,
		InputTokens:      r.InputTokens,
		OutputTokens:     r.OutputTokens,
		CacheReadTokens:  r.CacheReadTokens,
		CacheWriteTokens: r.CacheWriteTokens,
		UncachedTokens:   r.UncachedTokens,
		TokenEstimated:   r.Estimated,
		HitRate:          HitRate(r.InputTokens, r.CacheReadTokens),
	}
	if reported, err := parseFloatCost(r.CostEstimate); err == nil && reported > 0 {
		view.CostUSD = reported
	} else {
		view.CostUSD = EstimatedCostUSD(r)
		view.CostEstimated = true
	}
	return view
}

// HitRate is cache reads over all input tokens. Input is finalized by #17 to
// include cached tokens, so the fraction is comparable across providers.
func HitRate(input, cacheRead int64) float64 {
	if input <= 0 {
		return 0
	}
	return float64(cacheRead) / float64(input)
}
