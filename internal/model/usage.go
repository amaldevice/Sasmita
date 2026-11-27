package model

// FinalizeUsage fills in token counts a provider did not report directly.
// Values inferred from other reported counts are marked Estimated so the
// usage_records row never impersonates a provider measurement (TAD §13.3).
func FinalizeUsage(u Usage) Usage {
	if u.UncachedTokens <= 0 && u.InputTokens > 0 {
		uncached := u.InputTokens - u.CacheReadTokens - u.CacheWriteTokens
		if uncached < 0 {
			uncached = 0
			u.Estimated = true
		}
		u.UncachedTokens = uncached
	}
	if u.InputTokens == 0 && (u.CacheReadTokens > 0 || u.CacheWriteTokens > 0 || u.UncachedTokens > 0) {
		u.InputTokens = u.CacheReadTokens + u.CacheWriteTokens + u.UncachedTokens
		u.Estimated = true
	}
	return u
}
