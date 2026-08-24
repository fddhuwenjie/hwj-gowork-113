package itest

import (
    "testing"
    "germplasm/internal/domain"
)

func TestAnnotationBug23(t *testing.T) {
    e := newTestEnv(t)
    v1, err := e.svc.Rules.CreateRuleVersion(e.ctx, "tester", ruleInput("B23-RULE"))
    if err != nil { t.Fatal(err) }
    if _, err := e.svc.Rules.ActivateRule(e.ctx, "tester", v1.ID); err != nil { t.Fatal(err) }
    v2, err := e.svc.Rules.CreateRuleVersion(e.ctx, "tester", ruleInput("B23-RULE"))
    if err != nil { t.Fatal(err) }
    if _, err := e.svc.Rules.ActivateRule(e.ctx, "tester", v2.ID); err != nil { t.Fatal(err) }
    if _, err := e.svc.Rules.ActivateRule(e.ctx, "tester", v1.ID); err == nil { t.Fatal("retired rule was reported as reactivated") }
    active, err := e.svc.Repos.Rules.ActiveByCode(e.ctx, e.db, "B23-RULE")
    if err != nil { t.Fatalf("current active rule was lost: %v", err) }
    if active.ID != v2.ID || active.Status != domain.RuleActive { t.Fatalf("active rule changed: %+v", active) }
}

func TestAnnotationControlAnnotationBug23(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
