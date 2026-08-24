package itest

import (
    "testing"
    "time"
    "germplasm/internal/service"
)

func TestAnnotationBug28(t *testing.T) {
    e := newTestEnv(t)
    rule := e.setupRule()
    _, acc, batch, _ := e.setupBase(300, 100, "B28-C")
    e.setupSensors("B28-C", 2)
    out, err := e.svc.Outbound.Create(e.ctx, "tester", service.CreateOutboundInput{RequestNo:"B28-O", AccessionID:acc.ID, BatchID:batch.ID, Qty:120, RuleVersionID:rule.ID, Deadline:e.clk.Now().Add(24*time.Hour).Format(time.RFC3339Nano)})
    if err != nil { t.Fatal(err) }
    if _, err := e.svc.Outbound.Approve(e.ctx, "tester", out.ID, out.Version); err != nil { t.Fatal(err) }
    vars, err := e.svc.Risk.InventoryVariances(e.ctx)
    if err != nil { t.Fatal(err) }
    for _, v := range vars {
        if v.BatchID == batch.ID { t.Fatalf("valid frozen batch reported as variance: %+v", v) }
    }
}

func TestAnnotationControlAnnotationBug28(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
