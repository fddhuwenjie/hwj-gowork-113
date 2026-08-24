package itest

import (
    "testing"
    "time"
    "germplasm/internal/service"
)

func TestAnnotationBug06(t *testing.T) {
    e := newTestEnv(t)
    rule := e.setupRule()
    _, acc, batch, _ := e.setupBase(100, 50, "GOOD")
    e.setupSensors("GOOD", 2)
    e.setupSensorsWithValue("BAD", 2, 30, 30)
    o, err := e.svc.Outbound.Create(e.ctx, "tester", service.CreateOutboundInput{RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 50, RuleVersionID: rule.ID, Deadline: e.clk.Now().Add(24*time.Hour).Format(time.RFC3339Nano)})
    if err != nil { t.Fatal(err) }
    if _, err = e.svc.Outbound.Approve(e.ctx, "tester", o.ID, o.Version); err != nil { t.Fatalf("unrelated chamber polluted window: %v", err) }
}

func TestAnnotationControlAnnotationBug06(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
