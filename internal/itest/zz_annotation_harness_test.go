package itest

import (
    "testing"
    "germplasm/internal/service"
)

func TestAnnotationBug07(t *testing.T) {
    e := newTestEnv(t)
    plan := e.breedToTest(t, "A07")
    closed, err := e.svc.Breeding.ClosePlan(e.ctx, "tester", plan.ID, plan.Version)
    if err != nil { t.Fatal(err) }
    _, err = e.svc.Breeding.AddObservation(e.ctx, "tester", closed.ID, service.AddObservationInput{GerminationRate: 0.9, Vigor: "strong"})
    if err == nil { t.Fatal("closed plan accepted observation") }
    items, _ := e.svc.Breeding.ListObservations(e.ctx, plan.ID)
    if len(items) != 0 { t.Fatalf("observations=%d", len(items)) }
}

func TestAnnotationControlAnnotationBug07(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
