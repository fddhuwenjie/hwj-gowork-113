package itest

import (
	"testing"
	"time"

	"germplasm/internal/service"
)

func TestAnnotationBug05(t *testing.T) {
	e := newTestEnv(t)
	e.setupRule()
	e.clk.Advance(time.Second)
	narrow := ruleInput("STD-NARROW")
	narrow.MinTemp = -17
	narrow.MaxTemp = -16
	rv, err := e.svc.Rules.CreateRuleVersion(e.ctx, "tester", narrow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.svc.Rules.ActivateRule(e.ctx, "tester", rv.ID); err != nil {
		t.Fatal(err)
	}
	sensor, err := e.svc.Sensors.CreateSensor(e.ctx, "tester", service.CreateSensorInput{
		Code: e.unique("TEMP"), Chamber: "A05", Metric: "TEMPERATURE",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.svc.Sensors.AddReading(e.ctx, sensor.ID, service.AddReadingInput{
		Value: -18, RecordedAt: e.clk.Now().Add(-time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}
	risks, err := e.svc.Risk.LocationRisks(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, risk := range risks {
		if risk.Chamber == "A05" && risk.OutOfRangeCount > 0 {
			return
		}
	}
	t.Fatal("later active rule was ignored by risk aggregation")
}

func TestAnnotationControlAnnotationBug05(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
