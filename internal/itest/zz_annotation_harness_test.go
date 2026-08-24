package itest

import (
    "testing"
    "time"
    "germplasm/internal/domain"
    "germplasm/internal/service"
)

func TestAnnotationBug10(t *testing.T) {
    e := newTestEnv(t)
    chamber := "A10"
    rule := e.setupRule()
    _, acc, batch, _ := e.setupBase(500, 250, chamber)
    e.setupSensors(chamber, 2)
    refs := make(map[string]bool)
    for i := 0; i < 2; i++ {
        o, err := e.svc.Outbound.Create(e.ctx, "tester", service.CreateOutboundInput{RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 50, RuleVersionID: rule.ID, Deadline: e.clk.Now().Add(12*time.Hour).Format(time.RFC3339Nano)})
        if err != nil { t.Fatal(err) }
        if _, err = e.svc.Outbound.Approve(e.ctx, "tester", o.ID, o.Version); err != nil { t.Fatal(err) }
        refs[o.ID] = true
    }
    e.sched.RunOnceForTest(e.ctx)
    alerts, err := e.svc.Repos.Alerts.List(e.ctx, e.db, "OPEN", domain.AlertOutboundDueSoon, "", 50)
    if err != nil { t.Fatal(err) }
    found := 0
    for _, a := range alerts.Items { if refs[a.RefID] { found++ } }
    if found != 2 { t.Fatalf("due alerts=%d want=2", found) }
}

func TestAnnotationControlAnnotationBug10(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
