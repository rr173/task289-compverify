package service_test

import (
	"fmt"
	"sync"
	"testing"

	"task289-compverify/internal/ingest"
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

func TestConcurrentIngestDistinctEffectCount(t *testing.T) {
	app := newProbeApp(t)
	run, err := app.CreateRun("probe-concurrent-effects", "probe")
	if err != nil {
		t.Fatal(err)
	}
	ingestStep(t, app, run.ID, "reserve_stock", 1)
	const workers = 20
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 1; i <= workers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("effect.%d", n)
			if _, err := app.Ingest.IngestEffect(run.ID, ingest.EffectInput{
				StepName: "reserve_stock", EffectKey: key, Description: key, Gen: 1,
			}); err != nil {
				errCh <- fmt.Errorf("%s: %w", key, err)
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	got, err := app.Store.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EffCount != workers {
		t.Fatalf("eff_count=%d want=%d", got.EffCount, workers)
	}
}
