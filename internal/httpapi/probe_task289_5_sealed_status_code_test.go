package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"task289-compverify/internal/httpapi"
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

func TestSealedRunIngestEffectStatusCode(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/probe-sealed.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	app := service.New(st)
	run, err := app.CreateRun("sealed-probe", "probe")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.TransitionRun(run.ID, model.RunSealed); err != nil {
		t.Fatal(err)
	}
	h := httpapi.New(app, ":0").Handler()
	body, _ := json.Marshal(map[string]any{
		"step_name": "reserve_stock", "effect_key": "x.effect", "description": "x", "gen": 1,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs/"+itoa(run.ID)+"/effects", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["error"] != "SEALED" {
		t.Fatalf("error=%q want SEALED", resp["error"])
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
