package store

import (
	"database/sql"
	"errors"
	"time"

	"task289-compverify/internal/model"
)

const timeLayout = "2006-01-02T15:04:05.000Z07:00"

func fmtTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTime(s string) time.Time {
	t, _ := time.Parse(timeLayout, s)
	return t
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return fmtTime(*t)
}

// CreateRun 创建一次事务运行；trace_id 冲突时返回 CONFLICT。
func (s *Store) CreateRun(traceID, desc string) (model.Run, error) {
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`INSERT INTO runs(trace_id, description, status, created_at, updated_at) VALUES(?,?,?,?,?)`,
		traceID, desc, string(model.RunReceiving), fmtTime(now), fmtTime(now),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return model.Run{}, model.ErrConflict("trace_id already exists")
		}
		return model.Run{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetRun(id)
}

// GetRun 按 ID 读取运行。
func (s *Store) GetRun(id int64) (model.Run, error) {
	row := s.db.QueryRow(`SELECT id,trace_id,description,status,step_count,eff_count,comp_count,retry_count,created_at,updated_at FROM runs WHERE id=?`, id)
	var r model.Run
	var created, updated string
	err := row.Scan(&r.ID, &r.TraceID, &r.Description, &r.Status, &r.StepCount, &r.EffCount, &r.CompCount, &r.RetryCount, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Run{}, model.ErrNotFound("run", id)
	}
	if err != nil {
		return model.Run{}, err
	}
	r.CreatedAt, r.UpdatedAt = parseTime(created), parseTime(updated)
	return r, nil
}

// ListRuns 列出运行（按创建时间倒序）。
func (s *Store) ListRuns(limit int) ([]model.Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id,trace_id,description,status,step_count,eff_count,comp_count,retry_count,created_at,updated_at FROM runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Run
	for rows.Next() {
		var r model.Run
		var created, updated string
		if err := rows.Scan(&r.ID, &r.TraceID, &r.Description, &r.Status, &r.StepCount, &r.EffCount, &r.CompCount, &r.RetryCount, &created, &updated); err != nil {
			return nil, err
		}
		r.CreatedAt, r.UpdatedAt = parseTime(created), parseTime(updated)
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateRunStatus 迁移运行状态；非法迁移返回 ILLEGAL_TRANSITION。
func (s *Store) UpdateRunStatus(id int64, to model.RunStatus) error {
	r, err := s.GetRun(id)
	if err != nil {
		return err
	}
	if !model.CanTransition(r.Status, to) {
		return model.ErrIllegalTransition(r.Status, to)
	}
	now := fmtTime(time.Now().UTC())
	res, err := s.db.Exec(`UPDATE runs SET status=?, updated_at=? WHERE id=?`, string(to), now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound("run", id)
	}
	return nil
}

// SealRun 直接封存（任何非终态均可进入 sealed）。
func (s *Store) SealRun(id int64) error {
	now := fmtTime(time.Now().UTC())
	res, err := s.db.Exec(`UPDATE runs SET status=?, updated_at=? WHERE id=?`, string(model.RunSealed), now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound("run", id)
	}
	return nil
}

// SetRunStatusForVerify 由验证流水线驱动状态写入（绕过手动状态机，
// 因为验证本身就是在推进状态机），仅允许写入 closed / has_residue。
func (s *Store) SetRunStatusForVerify(id int64, status model.RunStatus) error {
	if status != model.RunClosed && status != model.RunHasResidue {
		return model.ErrBadInput("verify may only set closed or has_residue")
	}
	now := fmtTime(time.Now().UTC())
	res, err := s.db.Exec(`UPDATE runs SET status=?, updated_at=? WHERE id=?`, string(status), now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound("run", id)
	}
	return nil
}

// 事件计数：供 ingest 包在写入后更新运行上的聚合计数。
func (s *Store) bumpCounts(runID int64, steps, effs, comps, retries int) error {
	_, err := s.db.Exec(
		`UPDATE runs SET step_count=step_count+?, eff_count=eff_count+?, comp_count=comp_count+?, retry_count=retry_count+?, updated_at=? WHERE id=?`,
		steps, effs, comps, retries, fmtTime(time.Now().UTC()), runID,
	)
	return err
}

func isUniqueViolation(err error) bool {
	msg := err.Error()
	return containsAny(msg, "UNIQUE constraint", "unique constraint", "constraint failed")
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
