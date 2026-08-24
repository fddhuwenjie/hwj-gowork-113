package itest

import (
	"testing"
	"time"

	"germplasm/internal/domain"
	"germplasm/internal/service"
)

func TestAnnotationBug04(t *testing.T) {
	e := newTestEnv(t)
	e.setupRule()
	ids := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		sensor, err := e.svc.Sensors.CreateSensor(e.ctx, "tester", service.CreateSensorInput{
			Code: e.unique("TEMP"), Chamber: "A04", Metric: "TEMPERATURE",
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, sensor.ID)
		if _, err = e.svc.Sensors.AddReading(e.ctx, sensor.ID, service.AddReadingInput{
			Value: -10, RecordedAt: e.clk.Now().Add(-time.Minute).Format(time.RFC3339Nano),
		}); err != nil {
			t.Fatal(err)
		}
	}
	e.sched.RunOnceForTest(e.ctx)
	page, err := e.svc.Repos.Alerts.List(e.ctx, e.db, "OPEN", domain.AlertEnvOutOfRange, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, alert := range page.Items {
		if alert.RefType == "sensor" {
			seen[alert.RefID] = true
		}
	}
	if !seen[ids[0]] || !seen[ids[1]] {
		t.Fatalf("sensor alerts were merged or remapped: items=%+v", page.Items)
	}
}

func TestAnnotationControlAnnotationBug04(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
