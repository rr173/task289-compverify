package store_test

import (
	"testing"

	"task289-compverify/internal/store"
)

func TestCreateCompensationBumpsRunCounter(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/probe-comp-count.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	run, err := st.CreateRun("comp-count-probe", "probe")
	if err != nil {
		t.Fatal(err)
	}
	step, err := st.CreateStep(run.ID, "reserve_stock", 1)
	if err != nil {
		t.Fatal(err)
	}
	eff, err := st.CreateEffect(run.ID, step.ID, "stock.reserved", "hold stock", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateCompensation(run.ID, eff.ID, "release_stock", nil, 1); err != nil {
		t.Fatal(err)
	}
	eff2, err := st.CreateEffect(run.ID, step.ID, "balance.deducted", "deduct", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateCompensation(run.ID, eff2.ID, "refund_balance", nil, 1); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CompCount != 2 {
		t.Fatalf("comp_count=%d want=2", got.CompCount)
	}
}
