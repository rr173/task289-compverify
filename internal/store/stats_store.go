package store

import (
	"database/sql"

	"task289-compverify/internal/model"
)

// Stats 汇总一次运行的关键计数。
type Stats struct {
	Runs           int `json:"runs"`
	Steps          int `json:"steps"`
	Effects        int `json:"effects"`
	Compensations  int `json:"compensations"`
	Retries        int `json:"retries"`
	Snapshots      int `json:"snapshots"`
	OpenResidues   int `json:"open_residues"`
	ClosedRuns     int `json:"closed_runs"`
	SealedRuns     int `json:"sealed_runs"`
}

// GlobalStats 统计全库规模（供 /api/stats 与 smoke-test 使用）。
func (s *Store) GlobalStats() (Stats, error) {
	var st Stats
	one := func(q string, dst *int, args ...any) error {
		return s.db.QueryRow(q, args...).Scan(dst)
	}
	if err := one(`SELECT COUNT(*) FROM runs`, &st.Runs); err != nil {
		return st, err
	}
	if err := one(`SELECT COUNT(*) FROM steps`, &st.Steps); err != nil {
		return st, err
	}
	if err := one(`SELECT COUNT(*) FROM effects`, &st.Effects); err != nil {
		return st, err
	}
	if err := one(`SELECT COUNT(*) FROM compensations`, &st.Compensations); err != nil {
		return st, err
	}
	if err := one(`SELECT COUNT(*) FROM retries`, &st.Retries); err != nil {
		return st, err
	}
	if err := one(`SELECT COUNT(*) FROM snapshots`, &st.Snapshots); err != nil {
		return st, err
	}
	if err := one(`SELECT COUNT(*) FROM residues WHERE resolved=0`, &st.OpenResidues); err != nil {
		return st, err
	}
	if err := one(`SELECT COUNT(*) FROM runs WHERE status=?`, &st.ClosedRuns, string(model.RunClosed)); err != nil {
		return st, err
	}
	if err := one(`SELECT COUNT(*) FROM runs WHERE status=?`, &st.SealedRuns, string(model.RunSealed)); err != nil {
		return st, err
	}
	return st, nil
}

// Ping 供健康检查使用。
func (s *Store) Ping() error {
	return s.db.Ping()
}

// Exec 暴露原始执行（供 smoke-test 或诊断使用）。
func (s *Store) Exec(query string, args ...any) (sql.Result, error) {
	return s.db.Exec(query, args...)
}

// QueryRow 暴露原始查询。
func (s *Store) QueryRow(query string, args ...any) *sql.Row {
	return s.db.QueryRow(query, args...)
}
