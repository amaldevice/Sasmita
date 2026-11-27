package main

import (
	"context"
	"errors"
	"testing"

	"github.com/amaldevice/sasmita/internal/artifacts"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters"
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

// A fresh install has no provider configured. The shell must still open so
// onboarding can add one; runs fail closed with a plain-language error.
func TestDefaultProviderWithoutBaseURLFailsClosed(t *testing.T) {
	p := defaultProvider("")
	if p.Name() != "unconfigured" {
		t.Fatalf("Name() = %q, want unconfigured", p.Name())
	}
	err := p.Stream(context.Background(), model.StreamRequest{}, func(model.StreamEvent) {})
	if !errors.Is(err, adapters.ErrNoProviderConfigured) {
		t.Fatalf("Stream error = %v, want ErrNoProviderConfigured", err)
	}
}

func TestSetupSucceedsWithoutProviderEnv(t *testing.T) {
	t.Setenv("SASMITA_DB_PATH", t.TempDir()+"/sasmita.db")
	t.Setenv("SASMITA_OPENAI_BASE_URL", "")
	var shell shellService
	if err := setup(&shell); err != nil {
		t.Fatalf("setup with no provider env: %v", err)
	}
	if shell.bindings == nil || shell.app == nil {
		t.Fatal("setup did not initialise bindings and application services")
	}
}
