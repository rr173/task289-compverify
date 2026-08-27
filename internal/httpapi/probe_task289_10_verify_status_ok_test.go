package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"task289-compverify/internal/httpapi"
	"task289-compverify/internal/ingest"
	"task289-compverify/internal/model"
	"task289-compverify/internal/service"
	"task289-compverify/internal/store"
)

func newProbeServer(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/probe-http.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return httpapi.New(service.New(st), ":0").Handler()
}

func TestVerifyStatusNotOKWhenHasResidue(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/probe-verify-status.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	app := service.New(st)
	run, err := app.CreateRun("verify-status-probe", "probe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Ingest.IngestStep(run.ID, ingest.StepInput{Name: "reserve_stock", Ordinal: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Ingest.IngestStep(run.ID, ingest.StepInput{Name: "deduct_balance", Ordinal: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Ingest.IngestEffect(run.ID, ingest.EffectInput{StepName: "reserve_stock", EffectKey: "stock.reserved", Description: "a", Gen: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Ingest.IngestEffect(run.ID, ingest.EffectInput{StepName: "deduct_balance", EffectKey: "balance.deducted", Description: "b", Gen: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Ingest.IngestCompensation(run.ID, ingest.CompensationInput{EffectKey: "stock.reserved", Action: "release_stock", Gen: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.VerifyRun(run.ID); err != nil {
		t.Fatal(err)
	}
	h := httpapi.New(app, ":0").Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/runs/"+itoa(run.ID)+"/verify", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		OK       bool            `json:"ok"`
		Status   model.RunStatus `json:"status"`
		Residues int             `json:"residue_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.OK {
		t.Fatalf("expected ok=false for has_residue, got %+v", resp)
	}
	if resp.Status != model.RunHasResidue || resp.Residues == 0 {
		t.Fatalf("unexpected status payload: %+v", resp)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
