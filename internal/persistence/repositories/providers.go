package repositories

import (
	"context"
	"database/sql"

	"github.com/amaldevice/sasmita/internal/domain"
)

func (s *Store) CreateProviderProfile(ctx context.Context, p *domain.ProviderProfile) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO provider_profiles(id, provider_id, display_name, base_url, secret_ref, created_at, updated_at)
		 VALUES(?,?,?,?,?,?,?)`,
		p.ID, string(p.Kind), p.DisplayName, p.BaseURL, p.SecretRef, stamp(p.CreatedAt), stamp(p.UpdatedAt))
	return err
}

func (s *Store) GetProviderProfile(ctx context.Context, id string) (*domain.ProviderProfile, error) {
	var p domain.ProviderProfile
	var kind, created, updated string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, provider_id, display_name, base_url, secret_ref, created_at, updated_at
		 FROM provider_profiles WHERE id = ?`, id,
	).Scan(&p.ID, &kind, &p.DisplayName, &p.BaseURL, &p.SecretRef, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Kind, p.CreatedAt, p.UpdatedAt = domain.ProviderKind(kind), parseStamp(created), parseStamp(updated)
	return &p, nil
}

func (s *Store) ListProviderProfiles(ctx context.Context) ([]domain.ProviderProfile, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, provider_id, display_name, base_url, secret_ref, created_at, updated_at
		 FROM provider_profiles ORDER BY rowid ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ProviderProfile
	for rows.Next() {
		var p domain.ProviderProfile
		var kind, created, updated string
		if err := rows.Scan(&p.ID, &kind, &p.DisplayName, &p.BaseURL, &p.SecretRef, &created, &updated); err != nil {
			return nil, err
		}
		p.Kind, p.CreatedAt, p.UpdatedAt = domain.ProviderKind(kind), parseStamp(created), parseStamp(updated)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdateProviderProfile(ctx context.Context, p *domain.ProviderProfile) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE provider_profiles SET provider_id = ?, display_name = ?, base_url = ?, secret_ref = ?, updated_at = ?
		 WHERE id = ?`,
		string(p.Kind), p.DisplayName, p.BaseURL, p.SecretRef, stamp(p.UpdatedAt), p.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteProviderProfile(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM provider_profiles WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
