package itest

import (
    "testing"
    "germplasm/internal/service"
)

func TestAnnotationBug02(t *testing.T) {
    e := newTestEnv(t)
    _, acc, _, _ := e.setupBase(10, 10, "A02")
    b, err := e.svc.Storage.CreateOriginalBatch(e.ctx, "tester", service.CreateBatchInput{AccessionID: acc.ID, BatchNo: e.unique("CAP"), QtyTotal: 2})
    if err != nil { t.Fatal(err) }
    samples, err := e.svc.Storage.SplitSamples(e.ctx, "tester", service.SplitSamplesInput{BatchID: b.ID, Quantities: []int64{1, 1}})
    if err != nil { t.Fatal(err) }
    loc, err := e.svc.Storage.CreateLocation(e.ctx, "tester", service.CreateLocationInput{Code: e.unique("ONE"), Chamber: "A02", Capacity: 1})
    if err != nil { t.Fatal(err) }
    if _, err = e.svc.Storage.AssignLocation(e.ctx, "tester", samples[0].ID, loc.ID, samples[0].Version); err != nil { t.Fatal(err) }
    if _, err = e.svc.Storage.AssignLocation(e.ctx, "tester", samples[1].ID, loc.ID, samples[1].Version); err == nil { t.Fatal("full location accepted second sample") }
    locations, _ := e.svc.Storage.ListLocations(e.ctx, "A02", "", 50)
    for _, item := range locations.Items { if item.ID == loc.ID && item.Occupied != 1 { t.Fatalf("occupied=%d", item.Occupied) } }
}

func TestAnnotationControlAnnotationBug02(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
