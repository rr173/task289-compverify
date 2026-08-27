package verify

import (
	"testing"

	"task289-compverify/internal/causal"
	"task289-compverify/internal/model"
)

// TestVerifyFullCoverage 验证全补偿时报告 OK 且定级为 valid。
func TestVerifyFullCoverage(t *testing.T) {
	effects := []model.Effect{
		{ID: 1, EffectKey: "stock.reserved", Status: model.EffEffective, Gen: 1},
		{ID: 2, EffectKey: "balance.deducted", Status: model.EffEffective, Gen: 1},
	}
	comps := []model.Compensation{
		{ID: 1, RunID: 1, EffectID: 1, Action: "release_stock", DepsOn: nil, Status: model.CompCandidate},
		{ID: 2, RunID: 1, EffectID: 2, Action: "refund_balance", DepsOn: nil, Status: model.CompCandidate},
	}
	b := causal.NewBuilder(effects, map[int64]string{1: "reserve_stock", 2: "deduct_balance"})
	g := b.Build(1, effects, comps)
	rep := New().Verify(g)
	if !rep.OK {
		t.Fatalf("expected OK, got %s (uncovered=%d conflicts=%d)", rep.Message, rep.Uncovered, rep.OrderConflicts)
	}
	if rep.Covered != 2 {
		t.Errorf("expected 2 covered, got %d", rep.Covered)
	}
	if rep.CompUpdates[1] != model.CompValid || rep.CompUpdates[2] != model.CompValid {
		t.Errorf("candidates should be graded valid, got %v", rep.CompUpdates)
	}
}

// TestVerifyMissingCoverage 验证缺失补偿时报告未覆盖并给出遗留。
func TestVerifyMissingCoverage(t *testing.T) {
	effects := []model.Effect{
		{ID: 1, EffectKey: "stock.reserved", Status: model.EffEffective, Gen: 1},
		{ID: 2, EffectKey: "balance.deducted", Status: model.EffEffective, Gen: 1},
	}
	comps := []model.Compensation{
		{ID: 1, RunID: 1, EffectID: 1, Action: "release_stock", DepsOn: nil, Status: model.CompCandidate},
	}
	b := causal.NewBuilder(effects, map[int64]string{1: "reserve_stock", 2: "deduct_balance"})
	g := b.Build(1, effects, comps)
	rep := New().Verify(g)
	if rep.OK {
		t.Fatalf("expected not OK when balance.deducted uncovered")
	}
	if rep.Uncovered != 1 {
		t.Errorf("expected 1 uncovered, got %d", rep.Uncovered)
	}
	if len(rep.Residues) != 1 || rep.Residues[0].HeadEffect != "balance.deducted" {
		t.Errorf("expected residue for balance.deducted, got %+v", rep.Residues)
	}
}

// TestVerifyOrderConflict 验证补偿依赖成环时报告顺序冲突并定级 order_conflict。
func TestVerifyOrderConflict(t *testing.T) {
	effects := []model.Effect{
		{ID: 1, EffectKey: "a.effect", Status: model.EffEffective, Gen: 1},
		{ID: 2, EffectKey: "b.effect", Status: model.EffEffective, Gen: 1},
	}
	comps := []model.Compensation{
		{ID: 1, RunID: 1, EffectID: 1, Action: "comp_a", DepsOn: []string{"comp_b"}, Status: model.CompCandidate},
		{ID: 2, RunID: 1, EffectID: 2, Action: "comp_b", DepsOn: []string{"comp_a"}, Status: model.CompCandidate},
	}
	b := causal.NewBuilder(effects, map[int64]string{1: "step_a", 2: "step_b"})
	g := b.Build(1, effects, comps)
	rep := New().Verify(g)
	if rep.OK {
		t.Fatalf("expected not OK on dependency cycle")
	}
	if rep.OrderConflicts == 0 {
		t.Errorf("expected order conflicts detected")
	}
	if rep.CompUpdates[1] != model.CompOrderConflict || rep.CompUpdates[2] != model.CompOrderConflict {
		t.Errorf("cyclic compensations should be graded order_conflict, got %v", rep.CompUpdates)
	}
}

// TestGroupResidues 验证遗留路径合并去重。
func TestGroupResidues(t *testing.T) {
	in := []Residue{
		{Path: []int64{3}, HeadEffect: "c.effect", Cause: "uncovered_effect"},
		{Path: []int64{3}, HeadEffect: "c.effect", Cause: "uncovered_effect"},
		{Path: []int64{1}, HeadEffect: "a.effect", Cause: "uncovered_effect"},
	}
	out := GroupResidues(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 unique residues, got %d", len(out))
	}
	if out[0].HeadEffect != "a.effect" || out[1].HeadEffect != "c.effect" {
		t.Errorf("unexpected order: %+v", out)
	}
}
