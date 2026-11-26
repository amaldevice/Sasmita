package main

import (
	"testing"

	"github.com/amaldevice/sasmita/internal/artifacts"
	"github.com/amaldevice/sasmita/internal/model/adapters/openai"
)

// The shipped shell must enable automatic compaction: a zero budget makes
// ShouldCompact inert, however well-tested the compactor is in isolation.
func TestShippedConfigEnablesAutoCompaction(t *testing.T) {
	cfg := appConfig(openai.Adapter{ProviderID: "openai"}, "gpt-test", t.TempDir(), &artifacts.Service{})
	if cfg.InputBudgetTokens <= 0 {
		t.Fatalf("InputBudgetTokens = %d, want a real model input budget", cfg.InputBudgetTokens)
	}
	if cfg.CompactionThreshold <= 0 || cfg.CompactionThreshold > 1 {
		t.Fatalf("CompactionThreshold = %v, want a fraction of the input budget", cfg.CompactionThreshold)
	}
}
