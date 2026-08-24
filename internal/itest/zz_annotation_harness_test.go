package itest

import (
	"testing"
	"time"

	"germplasm/internal/service"
)

func TestAnnotationBug03(t *testing.T) {
	e := newTestEnv(t)
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(100, 50, "A03")
	e.setupSensors("A03", 2)
	o, err := e.svc.Outbound.Create(e.ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 50,
		RuleVersionID: rule.ID, Deadline: e.clk.Now().Add(24 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.svc.Outbound.Approve(e.ctx, "tester", o.ID, o.Version); err != nil {
		t.Fatal(err)
	}

	entries, _, err := e.svc.Audit.List(e.ctx, e.db, "outbound", o.ID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Action == "outbound.approve" {
			return
		}
	}
	t.Fatalf("outbound approval audit is not owned by request %s", o.ID)
}

func TestAnnotationControlAnnotationBug03(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
