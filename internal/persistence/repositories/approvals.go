package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/amaldevice/sasmita/internal/domain"
)

// CreateApproval stores the exact request the user is about to authorize. The
// binding columns are unique so a racing duplicate cannot create two waits.
func (s *Store) CreateApproval(ctx context.Context, a *domain.Approval) error {
	var resolved any
	if a.ResolvedAt != nil {
		resolved = stamp(*a.ResolvedAt)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO approvals(id, run_id, conversation_id, call_id, tool_id, mode, project_id,
		    status, decision, arguments_json, args_hash, policy_hash, action_text, resource_text,
		    impact_text, message, created_at, resolved_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.RunID, a.ConversationID, a.CallID, a.ToolID, a.Mode, a.ProjectID,
		a.Status, a.Decision, string(a.Arguments), a.ArgsHash, a.PolicyHash,
		a.Action, a.Resource, a.Impact, a.Message, stamp(a.CreatedAt), resolved)
	return err
}

func (s *Store) GetApproval(ctx context.Context, id string) (*domain.Approval, error) {
	return s.scanApproval(ctx, `SELECT id, run_id, conversation_id, call_id, tool_id, mode, project_id,
		status, decision, arguments_json, args_hash, policy_hash, action_text, resource_text,
		impact_text, message, created_at, resolved_at FROM approvals WHERE id = ?`, id)
}

// FindApproval returns the newest approval for one exact tool-call binding.
func (s *Store) FindApproval(ctx context.Context, runID, callID, argsHash, policyHash string) (*domain.Approval, error) {
	return s.scanApproval(ctx, `SELECT id, run_id, conversation_id, call_id, tool_id, mode, project_id,
		status, decision, arguments_json, args_hash, policy_hash, action_text, resource_text,
		impact_text, message, created_at, resolved_at FROM approvals
		WHERE run_id = ? AND call_id = ? AND args_hash = ? AND policy_hash = ?
		ORDER BY rowid DESC LIMIT 1`, runID, callID, argsHash, policyHash)
}

func (s *Store) scanApproval(ctx context.Context, query string, args ...any) (*domain.Approval, error) {
	var a domain.Approval
	var raw, created string
	var resolved sql.NullString
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&a.ID, &a.RunID, &a.ConversationID, &a.CallID, &a.ToolID, &a.Mode, &a.ProjectID,
		&a.Status, &a.Decision, &raw, &a.ArgsHash, &a.PolicyHash, &a.Action, &a.Resource,
		&a.Impact, &a.Message, &created, &resolved)
	if err == sql.ErrNoRows {
		return nil, domain.ErrApprovalNotFound
	}
	if err != nil {
		return nil, err
	}
	a.Arguments = json.RawMessage(raw)
	a.CreatedAt = parseStamp(created)
	if resolved.Valid {
		t := parseStamp(resolved.String)
		a.ResolvedAt = &t
	}
	return &a, nil
}

// ResolveApproval persists a first decision only. A second resolution cannot
// overwrite it, even if a stale UI still shows the card.
func (s *Store) ResolveApproval(ctx context.Context, id, status, decision string, at time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE approvals SET status = ?, decision = ?, resolved_at = ?
		 WHERE id = ? AND status = ?`, status, decision, stamp(at), id, domain.ApprovalPending)
	if err != nil {
		return err
	}
	return changed(res)
}

// SupersedeApproval marks an approval unusable when its request changed.
func (s *Store) SupersedeApproval(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE approvals SET status = ? WHERE id = ? AND status = ?`,
		domain.ApprovalSuperseded, id, domain.ApprovalPending)
	if err != nil {
		return err
	}
	return changed(res)
}

func (s *Store) ListApprovals(ctx context.Context, conversationID string) ([]domain.Approval, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, run_id, conversation_id, call_id, tool_id, mode, project_id,
		status, decision, arguments_json, args_hash, policy_hash, action_text, resource_text,
		impact_text, message, created_at, resolved_at FROM approvals
		WHERE conversation_id = ? ORDER BY rowid ASC`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Approval
	for rows.Next() {
		var a domain.Approval
		var raw, created string
		var resolved sql.NullString
		if err := rows.Scan(
			&a.ID, &a.RunID, &a.ConversationID, &a.CallID, &a.ToolID, &a.Mode, &a.ProjectID,
			&a.Status, &a.Decision, &raw, &a.ArgsHash, &a.PolicyHash, &a.Action, &a.Resource,
			&a.Impact, &a.Message, &created, &resolved); err != nil {
			return nil, err
		}
		a.Arguments = json.RawMessage(raw)
		a.CreatedAt = parseStamp(created)
		if resolved.Valid {
			t := parseStamp(resolved.String)
			a.ResolvedAt = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) AddGrant(ctx context.Context, g *domain.PolicyGrant) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO policy_grants(id, project_id, tool_id, resource_hash, policy_hash, created_at)
		VALUES(?,?,?,?,?,?)`, g.ID, g.ProjectID, g.ToolID, g.ResourceHash, g.PolicyHash, stamp(g.CreatedAt))
	return err
}

func (s *Store) HasGrant(ctx context.Context, projectID, toolID, resourceHash, policyHash string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM policy_grants
		WHERE project_id = ? AND tool_id = ? AND resource_hash = ? AND policy_hash = ? LIMIT 1`,
		projectID, toolID, resourceHash, policyHash).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ListGrants(ctx context.Context, projectID string) ([]domain.PolicyGrant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, tool_id, resource_hash, policy_hash, created_at
		FROM policy_grants WHERE project_id = ? ORDER BY rowid ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PolicyGrant
	for rows.Next() {
		var g domain.PolicyGrant
		var created string
		if err := rows.Scan(&g.ID, &g.ProjectID, &g.ToolID, &g.ResourceHash, &g.PolicyHash, &created); err != nil {
			return nil, err
		}
		g.CreatedAt = parseStamp(created)
		out = append(out, g)
	}
	return out, rows.Err()
}

// SaveCheckpoint/LoadCheckpoint implement the app-owned shape of Eino's
// Get/Set([]byte) store. Agent state and ADK state live under distinct keys.
func (s *Store) SaveCheckpoint(ctx context.Context, id string, data []byte) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO checkpoints(id, state_blob, updated_at) VALUES(?,?,?)
		 ON CONFLICT(id) DO UPDATE SET state_blob = excluded.state_blob, updated_at = excluded.updated_at`,
		id, data, stamp(time.Now().UTC()))
	return err
}

func (s *Store) LoadCheckpoint(ctx context.Context, id string) ([]byte, bool, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT state_blob FROM checkpoints WHERE id = ?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	return data, err == nil, err
}

func (s *Store) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	return s.LoadCheckpoint(ctx, checkPointID)
}

func (s *Store) Set(ctx context.Context, checkPointID string, data []byte) error {
	return s.SaveCheckpoint(ctx, checkPointID, data)
}
