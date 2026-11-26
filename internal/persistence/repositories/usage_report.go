package repositories

import (
	"context"

	"github.com/amaldevice/sasmita/internal/usage"
)

// ListUsageRecords feeds the Diagnostics read model. The calendar day comes
// from the run's start time: usage_records has no timestamp of its own, and a
// request belongs to the day its run began (TAD §13.3).
func (s *Store) ListUsageRecords(ctx context.Context) ([]usage.Record, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT date(r.started_at), u.provider_id, u.model_id, u.input_tokens, u.output_tokens,
		     u.cache_read_tokens, u.cache_write_tokens, u.uncached_tokens, u.estimated, u.cost_estimate
		 FROM usage_records AS u
		 JOIN runs AS r ON r.id = u.run_id
		 ORDER BY r.started_at ASC, u.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []usage.Record
	for rows.Next() {
		var rec usage.Record
		var estimated int
		if err := rows.Scan(&rec.Day, &rec.ProviderID, &rec.ModelID, &rec.InputTokens, &rec.OutputTokens,
			&rec.CacheReadTokens, &rec.CacheWriteTokens, &rec.UncachedTokens, &estimated, &rec.CostEstimate); err != nil {
			return nil, err
		}
		rec.Estimated = estimated != 0
		out = append(out, rec)
	}
	return out, rows.Err()
}
