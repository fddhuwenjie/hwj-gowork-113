package itest

import (
    "testing"
    "germplasm/internal/domain"
)

func TestAnnotationBug19(t *testing.T) {
    e := newTestEnv(t)
    e.setupRule()
    e.setupSensorsWithValue("B19-COLD", 0, -5, 30)
    e.sched.RunOnceForTest(e.ctx)
    first, err := e.svc.Repos.Alerts.List(e.ctx, e.db, "OPEN", domain.AlertEnvOutOfRange, "", 50)
    if err != nil || len(first.Items) == 0 { t.Fatalf("first=%+v err=%v", first, err) }
    alert := first.Items[0]
    if err := e.svc.Repos.Alerts.Ack(e.ctx, e.db, alert.ID, e.clk.Now()); err != nil { t.Fatal(err) }
    e.sched.RunOnceForTest(e.ctx)
    second, err := e.svc.Repos.Alerts.List(e.ctx, e.db, "OPEN", domain.AlertEnvOutOfRange, "", 50)
    if err != nil { t.Fatal(err) }
    for _, item := range second.Items { if item.RefID == alert.RefID { return } }
    t.Fatalf("persistent violation produced no new open alert: %+v", second.Items)
}

func TestAnnotationControlAnnotationBug19(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
