package itest

import (
    "testing"
    "time"
    "germplasm/internal/service"
)

func TestAnnotationBug16(t *testing.T) {
    e := newTestEnv(t)
    plan := e.breedToTest(t, "B16-COLD")
    e.clk.Advance(31 * 24 * time.Hour)
    e.sched.RunOnceForTest(e.ctx)
    current, err := e.svc.Breeding.GetPlan(e.ctx, plan.ID)
    if err != nil { t.Fatal(err) }
    if string(current.Status) != "TIMEOUT" { t.Fatalf("status=%s", current.Status) }
    if _, err := e.svc.Restock.Create(e.ctx, "tester", service.CreateRestockInput{
        RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 200,
    }); err == nil { t.Fatal("timed-out plan accepted for restock") }
}

func TestAnnotationControlAnnotationBug16(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
