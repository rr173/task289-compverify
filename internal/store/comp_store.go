package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"task289-compverify/internal/model"
)

// CreateCompensation 写入一条补偿边。自环返回 SELF_LOOP；效果不存在返回 UNKNOWN_EFFECT。
func (s *Store) CreateCompensation(runID, effectID int64, action string, deps []string, gen int) (model.Compensation, error) {
	if _, err := s.GetEffect(effectID); err != nil {
		return model.Compensation{}, model.ErrUnknownEffect()
	}
	for _, d := range deps {
		if d == action {
			return model.Compensation{}, model.ErrSelfLoopCompensation()
		}
	}
	depsJSON, _ := json.Marshal(deps)
	res, err := s.db.Exec(
		`INSERT INTO compensations(run_id, effect_id, action, deps_on, gen, status, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?)`,
		runID, effectID, action, string(depsJSON), gen, string(model.CompCandidate), fmtTime(time.Now().UTC()), fmtTime(time.Now().UTC()),
	)
	if err != nil {
		return model.Compensation{}, err
	}
	id, _ := res.LastInsertId()
	cp, err := s.GetCompensation(id)
	if err != nil {
		return model.Compensation{}, err
	}
	_ = s.bumpCounts(runID, 0, 0, 1, 0)
	return cp, nil
}

// GetCompensation 按 ID 读取补偿边。
func (s *Store) GetCompensation(id int64) (model.Compensation, error) {
	row := s.db.QueryRow(`SELECT id,run_id,effect_id,action,deps_on,gen,status,reason,created_at,updated_at FROM compensations WHERE id=?`, id)
	var cp model.Compensation
	var depsJSON, created, updated string
	err := row.Scan(&cp.ID, &cp.RunID, &cp.EffectID, &cp.Action, &depsJSON, &cp.Gen, &cp.Status, &cp.Reason, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Compensation{}, model.ErrNotFound("compensation", id)
	}
	if err != nil {
		return model.Compensation{}, err
	}
	_ = json.Unmarshal([]byte(depsJSON), &cp.DepsOn)
	cp.CreatedAt, cp.UpdatedAt = parseTime(created), parseTime(updated)
	return cp, nil
}

// ListCompensations 列出运行的全部补偿边。
func (s *Store) ListCompensations(runID int64) ([]model.Compensation, error) {
	rows, err := s.db.Query(`SELECT id,run_id,effect_id,action,deps_on,gen,status,reason,created_at,updated_at FROM compensations WHERE run_id=? ORDER BY id ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCompensations(rows)
}

func scanCompensations(rows *sql.Rows) ([]model.Compensation, error) {
	var out []model.Compensation
	for rows.Next() {
		var cp model.Compensation
		var depsJSON, created, updated string
		if err := rows.Scan(&cp.ID, &cp.RunID, &cp.EffectID, &cp.Action, &depsJSON, &cp.Gen, &cp.Status, &cp.Reason, &created, &updated); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(depsJSON), &cp.DepsOn)
		cp.CreatedAt, cp.UpdatedAt = parseTime(created), parseTime(updated)
		out = append(out, cp)
	}
	return out, rows.Err()
}

// UpdateCompensationStatus 更新补偿边状态。
func (s *Store) UpdateCompensationStatus(id int64, status model.CompensationStatus, reason string) error {
	res, err := s.db.Exec(`UPDATE compensations SET status=?, reason=?, updated_at=? WHERE id=?`, string(status), reason, fmtTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound("compensation", id)
	}
	return nil
}

// ListResidues 列出运行的全部遗留路径。
func (s *Store) ListResidues(runID int64) ([]model.Residue, error) {
	rows, err := s.db.Query(`SELECT id,run_id,path,head_effect,cause,resolved,resolve_note,created_at FROM residues WHERE run_id=? ORDER BY id ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Residue
	for rows.Next() {
		var r model.Residue
		var pathJSON, created string
		var resolved int
		if err := rows.Scan(&r.ID, &r.RunID, &pathJSON, &r.HeadEffect, &r.Cause, &resolved, &r.ResolveNote, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(pathJSON), &r.Path)
		r.Resolved = resolved == 1
		r.CreatedAt = parseTime(created)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReplaceResidues 清空并重写运行的遗留路径（验证结果刷新）。
func (s *Store) ReplaceResidues(runID int64, residues []model.Residue) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM residues WHERE run_id=?`, runID); err != nil {
		return err
	}
	for _, r := range residues {
		pathJSON, _ := json.Marshal(r.Path)
		if _, err := tx.Exec(
			`INSERT INTO residues(run_id, path, head_effect, cause, resolved, resolve_note, created_at) VALUES(?,?,?,?,0,'',?)`,
			runID, string(pathJSON), r.HeadEffect, r.Cause, fmtTime(time.Now().UTC()),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ResolveResidue 标记遗留为已处理。
func (s *Store) ResolveResidue(runID, residueID int64, note string) error {
	res, err := s.db.Exec(`UPDATE residues SET resolved=1, resolve_note=? WHERE id=? AND run_id=?`, note, residueID, runID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound("residue", residueID)
	}
	return nil
}

// splitDeps 解析逗号分隔的依赖（供查询层使用）。
func splitDeps(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
