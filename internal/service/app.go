// Package service 编排业务闭环：事件接收 → 因果图 → 完整性验证 → 遗留诊断 → 快照发布。
package service

import (
	"task289-compverify/internal/causal"
	"task289-compverify/internal/diagnose"
	"task289-compverify/internal/ingest"
	"task289-compverify/internal/model"
	"task289-compverify/internal/snapshot"
	"task289-compverify/internal/store"
	"task289-compverify/internal/verify"
)

// App 是领域服务的聚合根，持有全部依赖。
type App struct {
	Store    *store.Store
	Ingest   *ingest.Service
	Causal   *causal.Builder
	Verify   *verify.Verifier
	Diagnose *diagnose.Service
	Snapshot *snapshot.Service
}

// New 装配全部依赖。
func New(st *store.Store) *App {
	return &App{
		Store:    st,
		Ingest:   ingest.New(st),
		Verify:   verify.New(),
		Diagnose: diagnose.New(),
		Snapshot: snapshot.New(st),
	}
}

// CreateRun 创建一次事务运行。
func (a *App) CreateRun(traceID, desc string) (model.Run, error) {
	return a.Store.CreateRun(traceID, desc)
}

// TransitionRun 迁移运行状态（sealed 走专用路径）。
func (a *App) TransitionRun(id int64, to model.RunStatus) error {
	if to == model.RunSealed {
		return a.Store.SealRun(id)
	}
	return a.Store.UpdateRunStatus(id, to)
}

// LoadGraph 装载一次运行的完整因果图（效果 + 补偿边 + 步骤名）。
func (a *App) LoadGraph(runID int64) (*causal.Graph, error) {
	steps, err := a.Store.ListSteps(runID)
	if err != nil {
		return nil, err
	}
	stepNames := make(map[int64]string, len(steps))
	for _, st := range steps {
		stepNames[st.ID] = st.Name
	}
	effects, err := a.Store.ListEffects(runID)
	if err != nil {
		return nil, err
	}
	comps, err := a.Store.ListCompensations(runID)
	if err != nil {
		return nil, err
	}
	builder := causal.NewBuilder(effects, stepNames)
	return builder.Build(runID, effects, comps), nil
}

// VerifyRun 执行完整验证并持久化遗留路径，返回验证报告。
// 结果写入运行状态：通过 → closed，未通过 → has_residue。
func (a *App) VerifyRun(runID int64) (verify.Report, error) {
	g, err := a.LoadGraph(runID)
	if err != nil {
		return verify.Report{}, err
	}
	rep := a.Verify.Verify(g)
	// 应用验证动作的补偿边定级（candidate → valid / order_conflict）。
	for compID, status := range rep.CompUpdates {
		_ = a.Store.UpdateCompensationStatus(compID, status, "auto-graded by verification")
	}
	if rep.OK {
		_ = a.Store.ReplaceResidues(runID, nil)
		_ = a.Store.SetRunStatusForVerify(runID, model.RunClosed)
	} else {
		residues := a.Diagnose.Locate(g, rep)
		_ = a.Store.ReplaceResidues(runID, residues)
		_ = a.Store.SetRunStatusForVerify(runID, model.RunHasResidue)
	}
	return rep, nil
}

// ConfirmCompensation 工程师确认一条补偿边。
func (a *App) ConfirmCompensation(id int64) error {
	return a.Store.UpdateCompensationStatus(id, model.CompConfirmed, "engineer confirmed")
}

// WaiveCompensation 工程师豁免一条补偿边（外部不可补偿步骤）。
func (a *App) WaiveCompensation(id int64, reason string) error {
	return a.Store.UpdateCompensationStatus(id, model.CompWaived, reason)
}

// ResolveResidue 标记遗留路径已处理。
func (a *App) ResolveResidue(runID, residueID int64, note string) error {
	return a.Store.ResolveResidue(runID, residueID, note)
}

// PublishSnapshot 发布运行的最新草稿快照（或按 ID 发布），并写回图摘要。
func (a *App) PublishSnapshot(runID, snapID int64, summary string) (model.Snapshot, error) {
	g, err := a.LoadGraph(runID)
	if err != nil {
		return model.Snapshot{}, err
	}
	digest := store.DigestGraph(g.NodeListToEffects(), g.EdgesToCompensations(), g.ResiduesToModel())
	run, err := a.Store.GetRun(runID)
	if err != nil {
		return model.Snapshot{}, err
	}
	if summary == "" {
		summary = run.TraceID + " verification snapshot"
	}
	return a.Snapshot.Publish(runID, snapID, summary, digest)
}

// GlobalStats 汇总全库统计。
func (a *App) GlobalStats() (store.Stats, error) {
	return a.Store.GlobalStats()
}
