package itest

import (
    "testing"
    "germplasm/internal/service"
)

func TestAnnotationBug18(t *testing.T) {
    e := newTestEnv(t)
    for _, row := range []struct{ code, chamber string }{{"B18-A","B18-COLD"},{"B18-B","B18-COLD"},{"B18-C","B18-COLD"},{"B18-X","OTHER"}} {
        if _, err := e.svc.Sensors.CreateSensor(e.ctx, "tester", service.CreateSensorInput{Code:row.code, Chamber:row.chamber, Metric:"TEMPERATURE"}); err != nil { t.Fatal(err) }
    }
    first, err := e.svc.Sensors.ListSensors(e.ctx, "B18-COLD", "", 2)
    if err != nil || first.NextCursor == "" { t.Fatalf("first=%+v err=%v", first, err) }
    second, err := e.svc.Sensors.ListSensors(e.ctx, "B18-COLD", first.NextCursor, 2)
    if err != nil { t.Fatal(err) }
    seen := map[string]bool{}
    for _, item := range first.Items { seen[item.ID] = true }
    for _, item := range second.Items {
        if item.Chamber != "B18-COLD" || seen[item.ID] { t.Fatalf("unstable second page: first=%+v second=%+v", first.Items, second.Items) }
    }
}

func TestAnnotationControlAnnotationBug18(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
