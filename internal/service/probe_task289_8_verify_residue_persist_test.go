package service_test

import (
	"testing"

	"task289-compverify/internal/ingest"
	"task289-compverify/internal/model"
	"task289-compverify/internal/service"
	"task289-compverify/internal/store"
)

func newProbeApp(t *testing.T) *service.App {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/probe.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return service.New(st)
}

func ingestStep(t *testing.T, app *service.App, runID int64, name string, ord int) {
	t.Helper()
	if _, err := app.Ingest.IngestStep(runID, ingest.StepInput{Name: name, Ordinal: ord}); err != nil {
		t.Fatalf("ingest step %s: %v", name, err)
	}
}

func ingestEffect(t *testing.T, app *service.App, runID int64, step, key string) {
	t.Helper()
	if _, err := app.Ingest.IngestEffect(runID, ingest.EffectInput{
		StepName: step, EffectKey: key, Description: key, Gen: 1,
	}); err != nil {
		t.Fatalf("ingest effect %s: %v", key, err)
	}
}

func ingestComp(t *testing.T, app *service.App, runID int64, effectKey, action string, deps []string) {
	t.Helper()
	if _, err := app.Ingest.IngestCompensation(runID, ingest.CompensationInput{
		EffectKey: effectKey, Action: action, DepsOn: deps, Gen: 1,
	}); err != nil {
		t.Fatalf("ingest comp %s: %v", action, err)
	}
}

func TestVerifyRunPersistsResiduesOnFailure(t *testing.T) {
	app := newProbeApp(t)
	run, err := app.CreateRun("probe-residue-persist", "probe")
	if err != nil {
		t.Fatal(err)
	}
	ingestStep(t, app, run.ID, "reserve_stock", 1)
	ingestStep(t, app, run.ID, "deduct_balance", 2)
	ingestEffect(t, app, run.ID, "reserve_stock", "stock.reserved")
	ingestEffect(t, app, run.ID, "deduct_balance", "balance.deducted")
	ingestComp(t, app, run.ID, "stock.reserved", "release_stock", nil)
	rep, err := app.VerifyRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatal("expected verify failure")
	}
	got, err := app.Store.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.RunHasResidue {
		t.Fatalf("status=%q want=%q", got.Status, model.RunHasResidue)
	}
	residues, err := app.Store.ListResidues(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(residues) == 0 {
		t.Fatal("expected persisted residues")
	}
	if residues[0].HeadEffect != "balance.deducted" {
		t.Fatalf("head=%q want balance.deducted", residues[0].HeadEffect)
	}
}
