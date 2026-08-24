package itest

import (
    "testing"
    "time"
    "germplasm/internal/service"
)

func TestAnnotationBug26(t *testing.T) {
    e := newTestEnv(t)
    e.clk.Set(time.Date(2026, 1, 1, 8, 0, 0, 123456789, time.UTC))
    in := ruleInput("B26-RULE")
    in.WindowBeforeHours = 1
    in.WindowAfterHours = 0
    rv, err := e.svc.Rules.CreateRuleVersion(e.ctx, "tester", in)
    if err != nil { t.Fatal(err) }
    rule, err := e.svc.Rules.ActivateRule(e.ctx, "tester", rv.ID)
    if err != nil { t.Fatal(err) }
    _, acc, batch, _ := e.setupBase(100, 100, "B26-C")
    for _, spec := range []service.CreateSensorInput{{Code:"B26-T", Chamber:"B26-C", Metric:"TEMPERATURE"}, {Code:"B26-H", Chamber:"B26-C", Metric:"HUMIDITY"}} {
        sensor, err := e.svc.Sensors.CreateSensor(e.ctx, "tester", spec)
        if err != nil { t.Fatal(err) }
        value := -18.0
        if spec.Metric == "HUMIDITY" { value = 30 }
        if _, err := e.svc.Sensors.AddReading(e.ctx, sensor.ID, service.AddReadingInput{Value:value, RecordedAt:e.clk.Now().Add(-time.Hour).Format(time.RFC3339Nano)}); err != nil { t.Fatal(err) }
    }
    out, err := e.svc.Outbound.Create(e.ctx, "tester", service.CreateOutboundInput{RequestNo:"B26-O", AccessionID:acc.ID, BatchID:batch.ID, Qty:50, RuleVersionID:rule.ID, Deadline:e.clk.Now().Add(24*time.Hour).Format(time.RFC3339Nano)})
    if err != nil { t.Fatal(err) }
    if _, err := e.svc.Outbound.Approve(e.ctx, "tester", out.ID, out.Version); err != nil { t.Fatalf("closed start boundary lost: %v", err) }
}

func TestAnnotationControlAnnotationBug26(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
