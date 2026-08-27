package store

import (
	"database/sql"
	"errors"
	"time"

	"task289-compverify/internal/model"
)

// CreateStep 写入一个步骤。同 run 下步骤名唯一；重复返回 CONFLICT。
func (s *Store) CreateStep(runID int64, name string, ordinal int) (model.Step, error) {
	res, err := s.db.Exec(
		`INSERT INTO steps(run_id, name, ordinal, max_gen, created_at) VALUES(?,?,?,0,?)`,
		runID, name, ordinal, fmtTime(time.Now().UTC()),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return model.Step{}, model.ErrConflict("step name already exists in run")
		}
		return model.Step{}, err
	}
	id, _ := res.LastInsertId()
	st, err := s.GetStep(id)
	if err != nil {
		return model.Step{}, err
	}
	_ = s.bumpCounts(runID, 1, 0, 0, 0)
	return st, nil
}

// GetStep 按 ID 读取步骤。
func (s *Store) GetStep(id int64) (model.Step, error) {
	row := s.db.QueryRow(`SELECT id,run_id,name,ordinal,max_gen,created_at FROM steps WHERE id=?`, id)
	var st model.Step
	var created string
	err := row.Scan(&st.ID, &st.RunID, &st.Name, &st.Ordinal, &st.MaxGen, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Step{}, model.ErrNotFound("step", id)
	}
	if err != nil {
		return model.Step{}, err
	}
	st.CreatedAt = parseTime(created)
	return st, nil
}

// ListSteps 列出运行的全部步骤（按 ordinal 升序）。
func (s *Store) ListSteps(runID int64) ([]model.Step, error) {
	rows, err := s.db.Query(`SELECT id,run_id,name,ordinal,max_gen,created_at FROM steps WHERE run_id=? ORDER BY ordinal ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Step
	for rows.Next() {
		var st model.Step
		var created string
		if err := rows.Scan(&st.ID, &st.RunID, &st.Name, &st.Ordinal, &st.MaxGen, &created); err != nil {
			return nil, err
		}
		st.CreatedAt = parseTime(created)
		out = append(out, st)
	}
	return out, rows.Err()
}

// StepByName 按运行与名称查找步骤。
func (s *Store) StepByName(runID int64, name string) (model.Step, error) {
	row := s.db.QueryRow(`SELECT id,run_id,name,ordinal,max_gen,created_at FROM steps WHERE run_id=? AND name=?`, runID, name)
	var st model.Step
	var created string
	err := row.Scan(&st.ID, &st.RunID, &st.Name, &st.Ordinal, &st.MaxGen, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Step{}, model.ErrNotFound("step", 0)
	}
	if err != nil {
		return model.Step{}, err
	}
	st.CreatedAt = parseTime(created)
	return st, nil
}

// BumpStepGen 更新步骤的最大代次（只允许提升）。
func (s *Store) BumpStepGen(stepID int64, gen int) error {
	_, err := s.db.Exec(`UPDATE steps SET max_gen=? WHERE id=? AND max_gen < ?`, gen, stepID, gen)
	return err
}

// CreateEffect 写入效果记录；effect_key 在 run 内唯一，重复返回 DUPLICATE_EVENT。
func (s *Store) CreateEffect(runID, stepID int64, key, desc string, gen int) (model.Effect, error) {
	// 幂等：同 run 同 key 已存在则直接返回既有记录。
	var existing model.Effect
	var existingCreated string
	err := s.db.QueryRow(
		`SELECT id,run_id,step_id,effect_key,description,status,gen,created_at FROM effects WHERE run_id=? AND effect_key=?`,
		runID, key,
	).Scan(&existing.ID, &existing.RunID, &existing.StepID, &existing.EffectKey, &existing.Description, &existing.Status, &existing.Gen, &existingCreated)
	if err == nil {
		existing.CreatedAt = parseTime(existingCreated)
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return model.Effect{}, err
	}
	res, err := s.db.Exec(
		`INSERT INTO effects(run_id, step_id, effect_key, description, status, gen, created_at) VALUES(?,?,?,?,?,?,?)`,
		runID, stepID, key, desc, string(model.EffEffective), gen, fmtTime(time.Now().UTC()),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return model.Effect{}, model.ErrDuplicateEvent(key)
		}
		return model.Effect{}, err
	}
	id, _ := res.LastInsertId()
	ef, err := s.GetEffect(id)
	if err != nil {
		return model.Effect{}, err
	}
	_ = s.bumpCounts(runID, 0, 1, 0, 0)
	return ef, nil
}

// GetEffect 按 ID 读取效果。
func (s *Store) GetEffect(id int64) (model.Effect, error) {
	row := s.db.QueryRow(`SELECT id,run_id,step_id,effect_key,description,status,gen,created_at FROM effects WHERE id=?`, id)
	var ef model.Effect
	var created string
	err := row.Scan(&ef.ID, &ef.RunID, &ef.StepID, &ef.EffectKey, &ef.Description, &ef.Status, &ef.Gen, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Effect{}, model.ErrNotFound("effect", id)
	}
	if err != nil {
		return model.Effect{}, err
	}
	ef.CreatedAt = parseTime(created)
	return ef, nil
}

// ListEffects 列出运行的全部效果。
func (s *Store) ListEffects(runID int64) ([]model.Effect, error) {
	rows, err := s.db.Query(`SELECT id,run_id,step_id,effect_key,description,status,gen,created_at FROM effects WHERE run_id=? ORDER BY id ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Effect
	for rows.Next() {
		var ef model.Effect
		var created string
		if err := rows.Scan(&ef.ID, &ef.RunID, &ef.StepID, &ef.EffectKey, &ef.Description, &ef.Status, &ef.Gen, &created); err != nil {
			return nil, err
		}
		ef.CreatedAt = parseTime(created)
		out = append(out, ef)
	}
	return out, rows.Err()
}

// EffectByKey 按 run 与效果指纹查找效果。
func (s *Store) EffectByKey(runID int64, key string) (model.Effect, error) {
	row := s.db.QueryRow(`SELECT id,run_id,step_id,effect_key,description,status,gen,created_at FROM effects WHERE run_id=? AND effect_key=?`, runID, key)
	var ef model.Effect
	var created string
	err := row.Scan(&ef.ID, &ef.RunID, &ef.StepID, &ef.EffectKey, &ef.Description, &ef.Status, &ef.Gen, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Effect{}, model.ErrNotFound("effect", 0)
	}
	if err != nil {
		return model.Effect{}, err
	}
	ef.CreatedAt = parseTime(created)
	return ef, nil
}

// UpdateEffectStatus 更新效果状态（终态 residual 后可覆盖为 compensated，用于豁免场景）。
func (s *Store) UpdateEffectStatus(id int64, status model.EffectStatus) error {
	_, err := s.db.Exec(`UPDATE effects SET status=? WHERE id=?`, string(status), id)
	return err
}

// CreateRetry 记录一次重试；代次必须严格递增，否则返回 GEN_REGRESS。
func (s *Store) CreateRetry(runID, stepID int64, gen int, note string) (model.RetryEvent, error) {
	if gen <= 0 {
		return model.RetryEvent{}, model.ErrGenerationRegress()
	}
	var cur int
	err := s.db.QueryRow(`SELECT COALESCE(max(gen),0) FROM retries WHERE run_id=? AND step_id=?`, runID, stepID).Scan(&cur)
	if err != nil {
		return model.RetryEvent{}, err
	}
	if gen <= cur {
		return model.RetryEvent{}, model.ErrGenerationRegress()
	}
	res, err := s.db.Exec(
		`INSERT INTO retries(run_id, step_id, gen, triggered_at, note) VALUES(?,?,?,?,?)`,
		runID, stepID, gen, fmtTime(time.Now().UTC()), note,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return model.RetryEvent{}, model.ErrGenerationRegress()
		}
		return model.RetryEvent{}, err
	}
	id, _ := res.LastInsertId()
	_ = s.BumpStepGen(stepID, gen)
	_ = s.bumpCounts(runID, 0, 0, 0, 1)
	return model.RetryEvent{ID: id, RunID: runID, StepID: stepID, Gen: gen, Note: note, TriggeredAt: time.Now().UTC()}, nil
}
