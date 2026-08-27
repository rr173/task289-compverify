// Package store 提供 SQLite 持久化：建表迁移与各实体的 CRUD。
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store 封装 SQLite 连接与迁移。
type Store struct {
	db *sql.DB
}

// Open 打开（必要时创建）SQLite 数据库并执行建表迁移。
// dir 为空时使用临时目录，供 --smoke-test 使用。
func Open(dbPath string) (*Store, error) {
	if dbPath == "" {
		dbPath = filepath.Join(os.TempDir(), "compverify-smoke.db")
	}
	if dir := filepath.Dir(dbPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir db dir: %w", err)
		}
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(32)
	db.SetMaxIdleConns(32)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭底层连接。
func (s *Store) Close() error { return s.db.Close() }

// DB 暴露底层句柄，供事务性查询使用。
func (s *Store) DB() *sql.DB { return s.db }

// migrate 创建全部表结构（幂等）。
func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	trace_id TEXT NOT NULL UNIQUE,
	description TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'receiving',
	step_count INTEGER NOT NULL DEFAULT 0,
	eff_count INTEGER NOT NULL DEFAULT 0,
	comp_count INTEGER NOT NULL DEFAULT 0,
	retry_count INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS steps (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id INTEGER NOT NULL REFERENCES runs(id),
	name TEXT NOT NULL,
	ordinal INTEGER NOT NULL,
	max_gen INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	UNIQUE(run_id, name)
);

CREATE TABLE IF NOT EXISTS effects (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id INTEGER NOT NULL REFERENCES runs(id),
	step_id INTEGER NOT NULL REFERENCES steps(id),
	effect_key TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'pending',
	gen INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	UNIQUE(run_id, effect_key)
);

CREATE TABLE IF NOT EXISTS compensations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id INTEGER NOT NULL REFERENCES runs(id),
	effect_id INTEGER NOT NULL REFERENCES effects(id),
	action TEXT NOT NULL,
	deps_on TEXT NOT NULL DEFAULT '',
	gen INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'candidate',
	reason TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS retries (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id INTEGER NOT NULL REFERENCES runs(id),
	step_id INTEGER NOT NULL REFERENCES steps(id),
	gen INTEGER NOT NULL,
	triggered_at TEXT NOT NULL,
	note TEXT NOT NULL DEFAULT '',
	UNIQUE(run_id, step_id, gen)
);

CREATE TABLE IF NOT EXISTS snapshots (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id INTEGER NOT NULL REFERENCES runs(id),
	version INTEGER NOT NULL,
	status TEXT NOT NULL DEFAULT 'draft',
	summary TEXT NOT NULL DEFAULT '',
	graph_digest TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	published_at TEXT,
	UNIQUE(run_id, version)
);

CREATE TABLE IF NOT EXISTS residues (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id INTEGER NOT NULL REFERENCES runs(id),
	path TEXT NOT NULL,
	head_effect TEXT NOT NULL DEFAULT '',
	cause TEXT NOT NULL DEFAULT '',
	resolved INTEGER NOT NULL DEFAULT 0,
	resolve_note TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_steps_run ON steps(run_id);
CREATE INDEX IF NOT EXISTS idx_effects_run ON effects(run_id);
CREATE INDEX IF NOT EXISTS idx_effects_step ON effects(step_id);
CREATE INDEX IF NOT EXISTS idx_comps_run ON compensations(run_id);
CREATE INDEX IF NOT EXISTS idx_comps_effect ON compensations(effect_id);
CREATE INDEX IF NOT EXISTS idx_retries_run ON retries(run_id);
CREATE INDEX IF NOT EXISTS idx_snaps_run ON snapshots(run_id);
CREATE INDEX IF NOT EXISTS idx_residues_run ON residues(run_id);
`
	_, err := s.db.Exec(schema)
	return err
}
