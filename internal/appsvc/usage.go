package appsvc

import (
	"context"

	"github.com/amaldevice/sasmita/internal/usage"
)

// GetUsageReport is the read command behind Settings › Diagnostics. Estimated
// values stay flagged so the UI can label them; the chat read path never sees
// the hit-rate warning carried in the report (TAD §15.3).
func (s *Service) GetUsageReport(ctx context.Context) (usage.Report, error) {
	records, err := s.store.ListUsageRecords(ctx)
	if err != nil {
		return usage.Report{}, err
	}
	return usage.Aggregate(records), nil
}
