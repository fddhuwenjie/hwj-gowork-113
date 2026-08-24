package itest

import (
    "testing"
    "time"
    "germplasm/internal/service"
)

func TestAnnotationBug25(t *testing.T) {
    e := newTestEnv(t)
    rule := e.setupRule()
    _, acc, batch, _ := e.setupBase(300, 100, "B25-C")
    e.setupSensors("B25-C", 2)
    key := "B25-SHARED"
    in := service.CreateOutboundInput{RequestNo:"B25-O", AccessionID:acc.ID, BatchID:batch.ID, Qty:100, RuleVersionID:rule.ID, BreedingTarget:"扩繁", Deadline:e.clk.Now().Add(24*time.Hour).Format(time.RFC3339Nano), IdempotencyKey:key}
    out, err := e.svc.Outbound.Create(e.ctx, "tester", in)
    if err != nil { t.Fatal(err) }
    if _, err := e.svc.Outbound.Approve(e.ctx, "tester", out.ID, out.Version); err != nil { t.Fatal(err) }
    current, _ := e.svc.Outbound.Get(e.ctx, out.ID)
    if _, err := e.svc.Outbound.Fulfill(e.ctx, "tester", out.ID, current.Version); err != nil { t.Fatal(err) }
    plan, err := e.svc.Breeding.CreatePlan(e.ctx, "tester", service.CreatePlanInput{PlanNo:"B25-P", OutboundRequestID:out.ID, TargetQty:200, Deadline:e.clk.Now().Add(30*24*time.Hour).Format(time.RFC3339Nano)})
    if err != nil { t.Fatal(err) }
    crossEndpointErr := error(nil)
    if _, err := e.svc.Purity.CreateTest(e.ctx, "tester", service.CreateTestInput{PlanID:plan.ID, SampleQty:20, CoverageRatio:1, PurityRate:1, IdempotencyKey:key}); err != nil { crossEndpointErr = err }
    changed := in
    changed.Deadline = e.clk.Now().Add(48*time.Hour).Format(time.RFC3339Nano)
    _, changedErr := e.svc.Outbound.Create(e.ctx, "tester", changed)
    if crossEndpointErr != nil || changedErr == nil {
        t.Fatalf("idempotency isolation broken: cross_endpoint=%v changed_deadline=%v", crossEndpointErr, changedErr)
    }
}

func TestAnnotationControlAnnotationBug25(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
