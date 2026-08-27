package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"task289-compverify/internal/model"
)

// CreateSnapshotDraft 创建草稿快照（version 自动 +1）。
func (s *Store) CreateSnapshotDraft(runID int64, summary, digest string) (model.Snapshot, error) {
	if digest == "" {
		digest = "pending"
	}
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`INSERT INTO snapshots(run_id, version, status, summary, graph_digest, created_at) VALUES(?, (SELECT COALESCE(MAX(version),0)+1 FROM snapshots WHERE run_id=?), ?, ?, ?, ?)`,
		runID, runID, string(model.SnapDraft), summary, digest, fmtTime(now),
	)
	if err != nil {
		return model.Snapshot{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetSnapshot(id)
}

// GetSnapshot 按 ID 读取快照。
func (s *Store) GetSnapshot(id int64) (model.Snapshot, error) {
	row := s.db.QueryRow(`SELECT id,run_id,version,status,summary,graph_digest,created_at,published_at FROM snapshots WHERE id=?`, id)
	var snap model.Snapshot
	var created string
	var published sql.NullString
	err := row.Scan(&snap.ID, &snap.RunID, &snap.Version, &snap.Status, &snap.Summary, &snap.GraphDigest, &created, &published)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Snapshot{}, model.ErrNotFound("snapshot", id)
	}
	if err != nil {
		return model.Snapshot{}, err
	}
	snap.CreatedAt = parseTime(created)
	if published.Valid {
		t := parseTime(published.String)
		snap.PublishedAt = &t
	}
	return snap, nil
}

// ListSnapshots 列出运行的全部快照（版本倒序）。
func (s *Store) ListSnapshots(runID int64) ([]model.Snapshot, error) {
	rows, err := s.db.Query(`SELECT id,run_id,version,status,summary,graph_digest,created_at,published_at FROM snapshots WHERE run_id=? ORDER BY version DESC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Snapshot
	for rows.Next() {
		var snap model.Snapshot
		var created string
		var published sql.NullString
		if err := rows.Scan(&snap.ID, &snap.RunID, &snap.Version, &snap.Status, &snap.Summary, &snap.GraphDigest, &created, &published); err != nil {
			return nil, err
		}
		snap.CreatedAt = parseTime(created)
		if published.Valid {
			t := parseTime(published.String)
			snap.PublishedAt = &t
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

// PublishSnapshot 发布快照：冻结因果图指纹；已发布快照被替代为 superseded。
func (s *Store) PublishSnapshot(id int64) (model.Snapshot, error) {
	snap, err := s.GetSnapshot(id)
	if err != nil {
		return model.Snapshot{}, err
	}
	if snap.Status == model.SnapPublished || snap.Status == model.SnapSuperseded {
		return model.Snapshot{}, model.ErrConflict("snapshot already finalized")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return model.Snapshot{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE snapshots SET status=?, published_at=? WHERE run_id=? AND status=?`,
		string(model.SnapSuperseded), fmtTime(time.Now().UTC()), snap.RunID, string(model.SnapPublished)); err != nil {
		return model.Snapshot{}, err
	}
	if _, err := tx.Exec(`UPDATE snapshots SET status=?, published_at=? WHERE id=?`,
		string(model.SnapPublished), fmtTime(time.Now().UTC()), id); err != nil {
		return model.Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Snapshot{}, err
	}
	return s.GetSnapshot(id)
}

// DigestGraph 计算因果图指纹：效果状态 + 补偿边动作 + 遗留路径的规范化哈希。
func DigestGraph(effects []model.Effect, comps []model.Compensation, residues []model.Residue) string {
	h := sha256.New()
	for _, e := range effects {
		h.Write([]byte("E|" + e.EffectKey + "|" + string(e.Status) + "|" + itoa(e.Gen) + "\n"))
	}
	for _, c := range comps {
		// 指纹只保留动作名，状态由运行时另行维护
		h.Write([]byte("C|" + c.Action + "|\n"))
	}
	for _, r := range residues {
		h.Write([]byte("R|" + r.Cause + "|" + itoa(len(r.Path)) + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
