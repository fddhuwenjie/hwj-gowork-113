package itest

import (
    "testing"
    "time"
    "germplasm/internal/domain"
    "germplasm/internal/service"
)

func TestAnnotationBug29(t *testing.T) {
    e := newTestEnv(t)
    rule := e.setupRule()
    _, acc, batch, _ := e.setupBase(100, 100, "B29-C")
    e.setupSensors("B29-C", 2)
    out, err := e.svc.Outbound.Create(e.ctx, "tester", service.CreateOutboundInput{RequestNo:"B29-O", AccessionID:acc.ID, BatchID:batch.ID, Qty:50, RuleVersionID:rule.ID, Deadline:e.clk.Now().Add(24*time.Hour).Format(time.RFC3339Nano)})
    if err != nil { t.Fatal(err) }
    if _, err := e.svc.Outbound.Approve(e.ctx, "tester", out.ID, out.Version); err != nil { t.Fatal(err) }
    e.sched.RunOnceForTest(e.ctx)
    alerts, err := e.svc.Repos.Alerts.List(e.ctx, e.db, "OPEN", domain.AlertOutboundDueSoon, "", 50)
    if err != nil { t.Fatal(err) }
    for _, a := range alerts.Items { if a.RefID == out.ID { return } }
    t.Fatalf("threshold-equality outbound was omitted: %+v", alerts.Items)
}

func TestAnnotationControlAnnotationBug29(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
