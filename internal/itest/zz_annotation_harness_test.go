package itest

import "testing"

func TestAnnotationBug11(t *testing.T) {
    e := newTestEnv(t)
    plan := e.breedToTest(t, "B11-COLD")
    if _, err := e.db.ExecContext(e.ctx, `UPDATE breeding_plans SET plot=? WHERE id=?`, "PLOT-A7/ROW-2", plan.ID); err != nil {
        t.Fatal(err)
    }
    got, err := e.svc.Breeding.GetPlan(e.ctx, plan.ID)
    if err != nil { t.Fatal(err) }
    if got.BatchID != plan.BatchID || got.OutboundRequestID != plan.OutboundRequestID {
        t.Fatalf("plan linkage mutated: before=%+v after=%+v", plan, got)
    }
    if got.BatchID == got.OutboundRequestID { t.Fatalf("independent links collapsed: %+v", got) }
}

func TestAnnotationControlAnnotationBug11(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
