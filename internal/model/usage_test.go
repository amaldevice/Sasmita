package model_test

import (
	"testing"

	"github.com/amaldevice/sasmita/internal/model"
)

func TestFinalizeUsageDerivesUncachedTokens(t *testing.T) {
	cases := []struct {
		name       string
		in         model.Usage
		wantInput  int64
		wantUncach int64
		wantEst    bool
	}{
		{"openai total includes cached", model.Usage{InputTokens: 10, CacheReadTokens: 4, CacheWriteTokens: 2}, 10, 4, false},
		{"anthropic input already excludes cache", model.Usage{InputTokens: 7, CacheReadTokens: 2, CacheWriteTokens: 1, UncachedTokens: 7}, 7, 7, false},
		{"no cache fields is exact", model.Usage{InputTokens: 12}, 12, 12, false},
		{"only cache fields are reported", model.Usage{CacheReadTokens: 3}, 3, 0, true},
		{"inconsistent counts are marked estimated", model.Usage{InputTokens: 2, CacheReadTokens: 5}, 2, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := model.FinalizeUsage(tc.in)
			if got.InputTokens != tc.wantInput || got.UncachedTokens != tc.wantUncach || got.Estimated != tc.wantEst {
				t.Fatalf("FinalizeUsage = %+v, want input=%d uncached=%d estimated=%v",
					got, tc.wantInput, tc.wantUncach, tc.wantEst)
			}
		})
	}
}
