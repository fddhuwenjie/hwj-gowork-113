package itest

import (
    "testing"
    "germplasm/internal/domain"
    "germplasm/internal/service"
)

func TestAnnotationBug01(t *testing.T) {
    e := newTestEnv(t)
    plan := e.breedToTest(t, "A01")
    test, err := e.svc.Purity.CreateTest(e.ctx, "tester", service.CreateTestInput{PlanID: plan.ID, SampleQty: 100, CoverageRatio: 1, PurityRate: 0.99})
    if err != nil { t.Fatal(err) }
    if _, err = e.svc.Purity.SealTest(e.ctx, "tester", test.ID, test.Version); err != nil { t.Fatal(err) }
    rb, err := e.svc.Restock.Create(e.ctx, "tester", service.CreateRestockInput{RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 200})
    if err != nil { t.Fatal(err) }
    accepted, err := e.svc.Restock.Accept(e.ctx, "tester", rb.ID, rb.Version)
    if err != nil { t.Fatal(err) }
    _, err = e.svc.Restock.Reject(e.ctx, "tester", accepted.ID, "late review", accepted.Version)
    if err == nil { t.Fatal("accepted restock was rejected") }
    current, _ := e.svc.Restock.Get(e.ctx, rb.ID)
    if current.Status != domain.RestockAccepted || current.NewBatchID == "" { t.Fatalf("current=%+v", current) }
}

func TestAnnotationControlAnnotationBug01(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
