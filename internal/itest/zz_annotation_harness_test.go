package itest

import (
    "testing"
    "time"
    "germplasm/internal/service"
)

func TestAnnotationBug12(t *testing.T) {
    e := newTestEnv(t)
    plan := e.breedToTest(t, "B12-COLD")
    baseAt := e.clk.Now()
    rows := []struct{ offset time.Duration; rate float64 }{
        {3 * time.Hour, 0.70}, {time.Hour, 0.95}, {2 * time.Hour, 0.85},
    }
    for _, row := range rows {
        if _, err := e.svc.Breeding.AddObservation(e.ctx, "tester", plan.ID, service.AddObservationInput{
            ObservedAt: baseAt.Add(row.offset).Format(time.RFC3339Nano), GerminationRate: row.rate,
        }); err != nil { t.Fatal(err) }
        e.clk.Advance(time.Minute)
    }
    got, err := e.svc.Risk.GerminationDeclines(e.ctx)
    if err != nil { t.Fatal(err) }
    for _, item := range got {
        if item.PlanID == plan.ID && item.ConsecutiveDrops >= 2 { return }
    }
    t.Fatalf("business-time decline missing: %+v", got)
}

func TestAnnotationControlAnnotationBug12(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
