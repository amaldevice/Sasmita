package modes

import "github.com/amaldevice/sasmita/internal/domain"

// RereadWarningTokens is the threshold from TAD §9.3 / story #26: warn only
// when history is large enough that a re-read at full price matters.
const RereadWarningTokens = 4000

// SwitchEstimate is a preflight estimate, never a provider-reported usage
// figure (TAD §11: estimates must be marked as such).
type SwitchEstimate struct {
	EstimatedTokens int64 `json:"estimatedTokens"`
	Warn            bool  `json:"warn"`
	Estimated       bool  `json:"estimated"`
}

// EstimateSwitch returns a conservative character-based estimate of the tokens
// that must be re-read when the model or Mode changes. No tokenizer is assumed
// to exist for every model, so the app remains functional with unknown models.
func EstimateSwitch(history []domain.Message) SwitchEstimate {
	var chars int
	for _, m := range history {
		chars += 16 // message envelope overhead
		for _, part := range m.Content {
			chars += len(part.Text) + len(part.Raw)
		}
	}
	tokens := int64((chars + 3) / 4)
	return SwitchEstimate{
		EstimatedTokens: tokens,
		Warn:            tokens > RereadWarningTokens,
		Estimated:       true,
	}
}
