package snapshot_test

import (
	"testing"

	"task289-compverify/internal/service"
	"task289-compverify/internal/store"
)

func TestLatestPublishedReturnsPublishedSnapshot(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/probe-latest-published.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	app := service.New(st)
	run, err := app.CreateRun("latest-published-probe", "probe")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := app.Snapshot.CreateDraft(run.ID, "draft-v1")
	if err != nil {
		t.Fatal(err)
	}
	published, err := app.PublishSnapshot(run.ID, draft.ID, "published-v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Snapshot.CreateDraft(run.ID, "draft-v2"); err != nil {
		t.Fatal(err)
	}
	latest, err := app.Snapshot.LatestPublished(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil {
		t.Fatal("expected latest published snapshot")
	}
	if latest.Status != "published" {
		t.Fatalf("latest status=%q want published", latest.Status)
	}
	if latest.ID != published.ID {
		t.Fatalf("latest id=%d want %d", latest.ID, published.ID)
	}
}
