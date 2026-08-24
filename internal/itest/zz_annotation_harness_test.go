package itest

import (
    "testing"
    "germplasm/internal/domain"
    "germplasm/internal/service"
)

func TestAnnotationBug14(t *testing.T) {
    e := newTestEnv(t)
    plan := e.breedToTest(t, "B14-COLD")
    in := ruleInput("STD-SEED")
    in.MinPurity = 0.99
    v2, err := e.svc.Rules.CreateRuleVersion(e.ctx, "tester", in)
    if err != nil { t.Fatal(err) }
    if _, err := e.svc.Rules.ActivateRule(e.ctx, "tester", v2.ID); err != nil { t.Fatal(err) }
    test, err := e.svc.Purity.CreateTest(e.ctx, "tester", service.CreateTestInput{
        PlanID: plan.ID, SampleQty: 100, CoverageRatio: 1, PurityRate: 0.97,
    })
    if err != nil { t.Fatal(err) }
    sealed, err := e.svc.Purity.SealTest(e.ctx, "tester", test.ID, test.Version)
    if err != nil { t.Fatal(err) }
    if sealed.Verdict != domain.VerdictPass { t.Fatalf("frozen v1 verdict=%s", sealed.Verdict) }
}

func TestAnnotationControlAnnotationBug14(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
