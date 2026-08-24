package itest

import (
    "testing"
    "time"
    "germplasm/internal/domain"
    "germplasm/internal/service"
)

func TestAnnotationBug24(t *testing.T) {
    e := newTestEnv(t)
    rule := e.setupRule()
    _, acc, batch, _ := e.setupBase(200, 100, "B24-C")
    makeRejected := func(no string) string {
        o, err := e.svc.Outbound.Create(e.ctx, "tester", service.CreateOutboundInput{RequestNo:no, AccessionID:acc.ID, BatchID:batch.ID, Qty:20, RuleVersionID:rule.ID, Deadline:e.clk.Now().Add(24*time.Hour).Format(time.RFC3339Nano)})
        if err != nil { t.Fatal(err) }
        if _, err := e.svc.Outbound.Reject(e.ctx, "reviewer", o.ID, o.Version); err != nil { t.Fatal(err) }
        return o.ID
    }
    firstID := makeRejected("B24-O1")
    secondID := makeRejected("B24-O2")
    first, _ := e.svc.Outbound.Get(e.ctx, firstID)
    if _, err := e.svc.Outbound.Cancel(e.ctx, "tester", first.ID, first.Version); err == nil { t.Fatal("rejected request was cancelled") }
    for _, id := range []string{firstID, secondID} {
        got, err := e.svc.Outbound.Get(e.ctx, id)
        if err != nil { t.Fatal(err) }
        if got.Status != domain.OutboundRejected { t.Fatalf("terminal request %s changed: %+v", id, got) }
    }
}

func TestAnnotationControlAnnotationBug24(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
