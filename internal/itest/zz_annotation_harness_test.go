package itest

import (
    "testing"
    "germplasm/internal/service"
)

func TestAnnotationBug15(t *testing.T) {
    e := newTestEnv(t)
    plan := e.breedToTest(t, "B15-COLD")
    key := "shared-client-key"
    test, err := e.svc.Purity.CreateTest(e.ctx, "tester", service.CreateTestInput{
        PlanID: plan.ID, SampleQty: 100, CoverageRatio: 1, PurityRate: 0.98, IdempotencyKey: key,
    })
    if err != nil { t.Fatal(err) }
    restock, err := e.svc.Restock.Create(e.ctx, "tester", service.CreateRestockInput{
        RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 500, IdempotencyKey: key,
    })
    if err != nil { t.Fatalf("cross-endpoint key collided: %v", err) }
    if test.ID == restock.ID { t.Fatalf("independent endpoints replayed one object: %s", test.ID) }
}

func TestAnnotationControlAnnotationBug15(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
