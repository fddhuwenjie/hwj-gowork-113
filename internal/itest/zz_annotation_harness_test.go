package itest

import (
    "testing"
    "time"
    "germplasm/internal/service"
)

func TestAnnotationBug08(t *testing.T) {
    e := newTestEnv(t)
    plan := e.breedToTest(t, "A08")
    first, err := e.svc.Purity.CreateTest(e.ctx, "tester", service.CreateTestInput{PlanID: plan.ID, SampleQty: 100, CoverageRatio: 1, PurityRate: 0.99})
    if err != nil { t.Fatal(err) }
    e.clk.Advance(2 * time.Hour)
    second, err := e.svc.Purity.CreateTest(e.ctx, "tester", service.CreateTestInput{PlanID: plan.ID, SampleQty: 100, CoverageRatio: 1, PurityRate: 0.99})
    if err != nil { t.Fatal(err) }
    page, err := e.svc.Purity.ListTests(e.ctx, plan.ID, "", 10)
    if err != nil || len(page.Items) != 2 { t.Fatalf("page=%+v err=%v", page, err) }
    if page.Items[0].ID != first.ID || page.Items[1].ID != second.ID { t.Fatalf("order=%+v", page.Items) }
}

func TestAnnotationControlAnnotationBug08(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
