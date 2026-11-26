// Package repositories owns all SQL for the v1 core tables. Application
// services depend on Store, never on database/sql directly.
package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/amaldevice/sasmita/internal/domain"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseStamp(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func (s *Store) CreateProject(ctx context.Context, p *domain.Project) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO projects(id, name, root_uri, created_at, updated_at) VALUES(?,?,?,?,?)`,
		p.ID, p.Name, p.RootURI, stamp(p.CreatedAt), stamp(p.UpdatedAt))
	return err
}

func (s *Store) CreateConversation(ctx context.Context, c *domain.Conversation) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO conversations(id, project_id, title, active_harness_id, created_at, updated_at)
		 VALUES(?,?,?,?,?,?)`,
		c.ID, c.ProjectID, c.Title, c.ActiveHarnessID, stamp(c.CreatedAt), stamp(c.UpdatedAt))
	return err
}

func (s *Store) GetConversation(ctx context.Context, id string) (*domain.Conversation, error) {
	var c domain.Conversation
	var created, updated string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, title, active_harness_id, created_at, updated_at
		 FROM conversations WHERE id = ?`, id,
	).Scan(&c.ID, &c.ProjectID, &c.Title, &c.ActiveHarnessID, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt, c.UpdatedAt = parseStamp(created), parseStamp(updated)
	return &c, nil
}

// ListConversations uses keyset pagination. The cursor is the last row's
// updated_at and id, so ordering and the next cursor are stable.
func (s *Store) ListConversations(ctx context.Context, projectID, cursor string, limit int) ([]domain.Conversation, string, error) {
	return s.listPage(ctx, cursor, limit, `project_id = ?`, projectID)
}

// ListRecentConversations is the sidebar's cross-project "recent" view.
func (s *Store) ListRecentConversations(ctx context.Context, cursor string, limit int) ([]domain.Conversation, string, error) {
	return s.listPage(ctx, cursor, limit, "", nil)
}

func (s *Store) listPage(ctx context.Context, cursor string, limit int, where string, whereArg any) ([]domain.Conversation, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `SELECT id, project_id, title, active_harness_id, created_at, updated_at FROM conversations`
	args := []any{}
	if where != "" {
		query += ` WHERE ` + where
		args = append(args, whereArg)
	}
	if parts := strings.SplitN(cursor, "|", 2); len(parts) == 2 {
		if where == "" {
			query += ` WHERE (updated_at < ? OR (updated_at = ? AND id < ?))`
		} else {
			query += ` AND (updated_at < ? OR (updated_at = ? AND id < ?))`
		}
		args = append(args, parts[0], parts[0], parts[1])
	}
	query += ` ORDER BY updated_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var out []domain.Conversation
	for rows.Next() {
		var c domain.Conversation
		var created, updated string
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Title, &c.ActiveHarnessID, &created, &updated); err != nil {
			return nil, "", err
		}
		c.CreatedAt, c.UpdatedAt = parseStamp(created), parseStamp(updated)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		next = stamp(last.UpdatedAt) + "|" + last.ID
	}
	return out, next, nil
}

func (s *Store) TouchConversation(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE conversations SET updated_at = ? WHERE id = ?`, stamp(at), id)
	return err
}

func (s *Store) AddMessage(ctx context.Context, m *domain.Message) error {
	content, err := json.Marshal(m.Content)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO messages(id, conversation_id, role, content_json, created_at) VALUES(?,?,?,?,?)`,
		m.ID, m.ConversationID, string(m.Role), string(content), stamp(m.CreatedAt))
	return err
}

// ListMessages returns messages in canonical (append) order. When limit > 0,
// the most recent `limit` messages are kept, still in append order.
func (s *Store) ListMessages(ctx context.Context, conversationID string, limit int) ([]domain.Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, conversation_id, role, content_json, created_at
		 FROM messages WHERE conversation_id = ? ORDER BY rowid ASC`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Message
	for rows.Next() {
		var m domain.Message
		var raw, created string
		if err := rows.Scan(&m.ID, &m.ConversationID, (*string)(&m.Role), &raw, &created); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &m.Content); err != nil {
			return nil, fmt.Errorf("message %s content: %w", m.ID, err)
		}
		m.CreatedAt = parseStamp(created)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (s *Store) CreateRun(ctx context.Context, r *domain.Run) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runs(id, conversation_id, status, model_ref, harness_id, started_at, finished_at)
		 VALUES(?,?,?,?,?,?,NULL)`,
		r.ID, r.ConversationID, string(r.Status), r.ModelRef, r.HarnessID, stamp(r.StartedAt))
	return err
}

// UpdateRunStatus persists a transition. A terminal state is never overwritten
// silently (TAD §8.3).
func (s *Store) UpdateRunStatus(ctx context.Context, runID string, status domain.RunStatus, finishedAt *time.Time) error {
	var finished any
	if finishedAt != nil {
		finished = stamp(*finishedAt)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE runs SET status = ?, finished_at = ?
		 WHERE id = ? AND status NOT IN ('completed','failed','cancelled')`,
		string(status), finished, runID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrTerminalRun
	}
	return nil
}

func (s *Store) GetRun(ctx context.Context, id string) (*domain.Run, error) {
	var r domain.Run
	var status, started string
	var finished sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, conversation_id, status, model_ref, harness_id, started_at, finished_at
		 FROM runs WHERE id = ?`, id,
	).Scan(&r.ID, &r.ConversationID, &status, &r.ModelRef, &r.HarnessID, &started, &finished)
	if err == sql.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.Status, r.StartedAt = domain.RunStatus(status), parseStamp(started)
	if finished.Valid {
		t := parseStamp(finished.String)
		r.FinishedAt = &t
	}
	return &r, nil
}

// AppendRunEvent assigns the next monotonic sequence number inside a
// transaction and returns the fully populated event for immediate IPC delivery.
func (s *Store) AppendRunEvent(ctx context.Context, runID, eventType string, payload json.RawMessage) (domain.RunEvent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.RunEvent{}, err
	}
	var seq uint64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM run_events WHERE run_id = ?`, runID,
	).Scan(&seq); err != nil {
		tx.Rollback()
		return domain.RunEvent{}, err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO run_events(run_id, seq, event_type, payload_json, created_at) VALUES(?,?,?,?,?)`,
		runID, seq, eventType, string(payload), stamp(now),
	); err != nil {
		tx.Rollback()
		return domain.RunEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.RunEvent{}, err
	}
	return domain.RunEvent{RunID: runID, Seq: seq, Type: eventType, Timestamp: now, Payload: payload}, nil
}

func (s *Store) ListRunEvents(ctx context.Context, runID string) ([]domain.RunEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT run_id, seq, event_type, payload_json, created_at
		 FROM run_events WHERE run_id = ? ORDER BY seq ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.RunEvent
	for rows.Next() {
		var ev domain.RunEvent
		var typ, payload, created string
		if err := rows.Scan(&ev.RunID, &ev.Seq, &typ, &payload, &created); err != nil {
			return nil, err
		}
		ev.Type, ev.Payload, ev.Timestamp = typ, json.RawMessage(payload), parseStamp(created)
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *Store) AddUsage(ctx context.Context, runID string, u domain.Usage) error {
	estimated := 0
	if u.Estimated {
		estimated = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO usage_records(run_id, provider_id, model_id, input_tokens, output_tokens,
		     cache_read_tokens, cache_write_tokens, uncached_tokens, estimated, cost_estimate)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`,
		runID, u.ProviderID, u.ModelID, u.InputTokens, u.OutputTokens,
		u.CacheReadTokens, u.CacheWriteTokens, u.UncachedTokens, estimated, u.CostEstimate)
	return err
}

// StartRunTx persists the run record and the user message atomically, before
// any remote request is made (TAD §20).
func (s *Store) StartRunTx(ctx context.Context, run *domain.Run, msg *domain.Message, touchedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	content, err := json.Marshal(msg.Content)
	if err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO runs(id, conversation_id, status, model_ref, harness_id, started_at, finished_at)
		 VALUES(?,?,?,?,?,?,NULL)`,
		run.ID, run.ConversationID, string(run.Status), run.ModelRef, run.HarnessID, stamp(run.StartedAt),
	); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO messages(id, conversation_id, role, content_json, created_at) VALUES(?,?,?,?,?)`,
		msg.ID, msg.ConversationID, string(msg.Role), string(content), stamp(msg.CreatedAt),
	); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE conversations SET updated_at = ? WHERE id = ?`, stamp(touchedAt), run.ConversationID,
	); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) ListRuns(ctx context.Context, conversationID string) ([]domain.Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, conversation_id, status, model_ref, harness_id, started_at, finished_at
		 FROM runs WHERE conversation_id = ? ORDER BY rowid ASC`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Run
	for rows.Next() {
		var r domain.Run
		var status, started string
		var finished sql.NullString
		if err := rows.Scan(&r.ID, &r.ConversationID, &status, &r.ModelRef, &r.HarnessID, &started, &finished); err != nil {
			return nil, err
		}
		r.Status, r.StartedAt = domain.RunStatus(status), parseStamp(started)
		if finished.Valid {
			t := parseStamp(finished.String)
			r.FinishedAt = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ftsQuery turns free-form user input into a safe FTS5 MATCH expression. Each
// token is quoted (so punctuation cannot be read as FTS syntax) and made a
// prefix, so "conv" matches "conversation" in either Indonesian or English.
func ftsQuery(raw string) string {
	tokens := strings.FieldsFunc(raw, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	terms := make([]string, 0, len(tokens))
	for _, token := range tokens {
		quoted := `"` + strings.ReplaceAll(token, `"`, `""`) + `"*`
		terms = append(terms, quoted)
	}
	return strings.Join(terms, " AND ")
}

// SearchConversations ranks hits across titles and message text. One
// conversation may have several FTS rows, so results are de-duplicated keeping
// the best (lowest bm25) rank first.
func (s *Store) SearchConversations(ctx context.Context, raw string, limit int) ([]domain.SearchResult, error) {
	match := ftsQuery(raw)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.conversation_id, c.title, c.project_id, bm25(conversation_fts) AS rank,
		       snippet(conversation_fts, 3, '', '', '…', 12) AS snippet
		FROM conversation_fts AS f
		JOIN conversations AS c ON c.id = f.conversation_id
		WHERE conversation_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, match, limit*4)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.SearchResult
	seen := make(map[string]bool)
	for rows.Next() {
		var r domain.SearchResult
		var rank float64
		if err := rows.Scan(&r.ConversationID, &r.Title, &r.ProjectID, &rank, &r.Snippet); err != nil {
			return nil, err
		}
		if seen[r.ConversationID] {
			continue
		}
		seen[r.ConversationID] = true
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

func (s *Store) RenameConversation(ctx context.Context, id, title string, at time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE conversations SET title = ?, updated_at = ? WHERE id = ?`, title, stamp(at), id)
	if err != nil {
		return err
	}
	return changed(res)
}

func (s *Store) MoveConversation(ctx context.Context, id, projectID string, at time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE conversations SET project_id = ?, updated_at = ? WHERE id = ?`, projectID, stamp(at), id)
	if err != nil {
		return err
	}
	return changed(res)
}

// ListProjects omits the internal no-project sentinel row.
func (s *Store) ListProjects(ctx context.Context) ([]domain.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, root_uri, created_at, updated_at
		 FROM projects WHERE id <> '' ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		var p domain.Project
		var created, updated string
		if err := rows.Scan(&p.ID, &p.Name, &p.RootURI, &created, &updated); err != nil {
			return nil, err
		}
		p.CreatedAt, p.UpdatedAt = parseStamp(created), parseStamp(updated)
		out = append(out, p)
	}
	return out, rows.Err()
}

func changed(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) ListUsage(ctx context.Context, runID string) ([]domain.Usage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT provider_id, model_id, input_tokens, output_tokens, cache_read_tokens,
		     cache_write_tokens, uncached_tokens, estimated, cost_estimate
		 FROM usage_records WHERE run_id = ? ORDER BY id ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Usage
	for rows.Next() {
		var u domain.Usage
		var estimated int
		if err := rows.Scan(&u.ProviderID, &u.ModelID, &u.InputTokens, &u.OutputTokens,
			&u.CacheReadTokens, &u.CacheWriteTokens, &u.UncachedTokens, &estimated, &u.CostEstimate); err != nil {
			return nil, err
		}
		u.Estimated = estimated != 0
		out = append(out, u)
	}
	return out, rows.Err()
}
