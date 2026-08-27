package verify_test

import (
	"testing"

	"task289-compverify/internal/causal"
	"task289-compverify/internal/model"
	"task289-compverify/internal/verify"
)

func TestSummarizeRejectsOrderConflictsWhenCovered(t *testing.T) {
	effects := []model.Effect{
		{ID: 1, EffectKey: "a.effect", Status: model.EffEffective, Gen: 1},
		{ID: 2, EffectKey: "b.effect", Status: model.EffEffective, Gen: 1},
	}
	comps := []model.Compensation{
		{ID: 1, RunID: 1, EffectID: 1, Action: "comp_a", DepsOn: []string{"comp_b"}, Status: model.CompValid},
		{ID: 2, RunID: 1, EffectID: 2, Action: "comp_b", DepsOn: []string{"comp_a"}, Status: model.CompValid},
	}
	b := causal.NewBuilder(effects, map[int64]string{1: "step_a", 2: "step_b"})
	g := b.Build(1, effects, comps)
	rep := verify.New().Verify(g)
	if rep.OK {
		t.Fatal("expected not OK when dependency cycle exists even if effects are covered")
	}
	if rep.OrderConflicts == 0 {
		t.Fatal("expected order conflicts to be reported")
	}
}
