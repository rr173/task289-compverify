package causal

import (
	"testing"

	"task289-compverify/internal/model"
)

// TestTopologicalActions 验证补偿动作逆依赖的拓扑排序与环检测。
func TestTopologicalActions(t *testing.T) {
	effects := []model.Effect{
		{ID: 1, EffectKey: "stock.reserved", Status: model.EffEffective, Gen: 1},
		{ID: 2, EffectKey: "balance.deducted", Status: model.EffEffective, Gen: 1},
		{ID: 3, EffectKey: "order.dispatched", Status: model.EffEffective, Gen: 1},
	}
	comps := []model.Compensation{
		{ID: 1, RunID: 1, EffectID: 1, Action: "release_stock", DepsOn: nil, Status: model.CompCandidate},
		{ID: 2, RunID: 1, EffectID: 2, Action: "refund_balance", DepsOn: []string{"release_stock"}, Status: model.CompCandidate},
		{ID: 3, RunID: 1, EffectID: 3, Action: "cancel_dispatch", DepsOn: []string{"refund_balance"}, Status: model.CompCandidate},
	}
	b := NewBuilder(effects, map[int64]string{1: "reserve_stock", 2: "deduct_balance", 3: "dispatch_order"})
	g := b.Build(1, effects, comps)
	order, hasCycle, cycle := g.TopologicalActions()
	if hasCycle {
		t.Fatalf("unexpected cycle: %v", cycle)
	}
	if len(order) != 3 {
		t.Fatalf("expected 3 actions in topo order, got %d: %v", len(order), order)
	}
	// 校验逆依赖序：release_stock 必须先于 refund_balance，refund_balance 先于 cancel_dispatch。
	pos := make(map[string]int)
	for i, a := range order {
		pos[a] = i
	}
	if pos["release_stock"] >= pos["refund_balance"] {
		t.Errorf("release_stock must precede refund_balance, order=%v", order)
	}
	if pos["refund_balance"] >= pos["cancel_dispatch"] {
		t.Errorf("refund_balance must precede cancel_dispatch, order=%v", order)
	}
}

// TestTopologicalCycle 验证补偿依赖成环时能检测出来。
func TestTopologicalCycle(t *testing.T) {
	effects := []model.Effect{
		{ID: 1, EffectKey: "a.effect", Status: model.EffEffective},
		{ID: 2, EffectKey: "b.effect", Status: model.EffEffective},
	}
	comps := []model.Compensation{
		{ID: 1, RunID: 1, EffectID: 1, Action: "comp_a", DepsOn: []string{"comp_b"}, Status: model.CompCandidate},
		{ID: 2, RunID: 1, EffectID: 2, Action: "comp_b", DepsOn: []string{"comp_a"}, Status: model.CompCandidate},
	}
	b := NewBuilder(effects, map[int64]string{1: "step_a", 2: "step_b"})
	g := b.Build(1, effects, comps)
	_, hasCycle, cycle := g.TopologicalActions()
	if !hasCycle {
		t.Fatalf("expected cycle detection, got none")
	}
	if len(cycle) == 0 {
		t.Fatalf("expected non-empty cycle members")
	}
}
