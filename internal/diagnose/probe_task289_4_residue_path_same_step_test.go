package diagnose_test

import (
	"testing"

	"task289-compverify/internal/causal"
	"task289-compverify/internal/diagnose"
	"task289-compverify/internal/model"
	"task289-compverify/internal/verify"
)

func TestLocateIncludesSameStepPredecessors(t *testing.T) {
	effects := []model.Effect{
		{ID: 1, EffectKey: "step.hold", Status: model.EffEffective, Gen: 1},
		{ID: 2, EffectKey: "step.charge", Status: model.EffEffective, Gen: 1},
	}
	comps := []model.Compensation{
		{ID: 1, RunID: 1, EffectID: 1, Action: "release_hold", Status: model.CompValid},
	}
	b := causal.NewBuilder(effects, map[int64]string{1: "pay_step", 2: "pay_step"})
	g := b.Build(1, effects, comps)
	rep := verify.New().Verify(g)
	if rep.OK {
		t.Fatal("expected uncovered effect")
	}
	out := diagnose.New().Locate(g, rep)
	if len(out) != 1 {
		t.Fatalf("expected 1 residue, got %d", len(out))
	}
	if len(out[0].Path) != 2 {
		t.Fatalf("path=%v want 2 effect ids for same-step predecessor", out[0].Path)
	}
}
