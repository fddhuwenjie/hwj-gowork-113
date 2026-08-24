package itest

import (
    "testing"
    "time"
    "germplasm/internal/service"
)

func TestAnnotationBug20(t *testing.T) {
    e := newTestEnv(t)
    rule := e.setupRule()
    _, acc, batch, _ := e.setupBase(500, 100, "B20-COLD")
    e.setupSensors("OTHER-COLD", 2)
    out, err := e.svc.Outbound.Create(e.ctx, "tester", service.CreateOutboundInput{
        RequestNo:e.unique("OUT"), AccessionID:acc.ID, BatchID:batch.ID, Qty:100,
        RuleVersionID:rule.ID, Deadline:e.clk.Now().Add(24*time.Hour).Format(time.RFC3339Nano),
    })
    if err != nil { t.Fatal(err) }
    if _, err := e.svc.Outbound.Approve(e.ctx, "tester", out.ID, out.Version); err == nil {
        t.Fatal("foreign chamber readings satisfied target window")
    }
}

func TestAnnotationControlAnnotationBug20(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
