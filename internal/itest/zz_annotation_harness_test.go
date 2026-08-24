package itest

import (
    "testing"
    "germplasm/internal/service"
)

func TestAnnotationBug22(t *testing.T) {
    e := newTestEnv(t)
    r, err := e.svc.Resources.CreateResource(e.ctx, "tester", service.CreateResourceInput{Code:"B22-R", Name:"水稻", Species:"Oryza sativa"})
    if err != nil { t.Fatal(err) }
    a, err := e.svc.Resources.CreateAccession(e.ctx, "tester", service.CreateAccessionInput{ResourceID:r.ID, AccessionNo:"B22-A"})
    if err != nil { t.Fatal(err) }
    b, err := e.svc.Storage.CreateOriginalBatch(e.ctx, "tester", service.CreateBatchInput{AccessionID:a.ID, BatchNo:"B22-B", QtyTotal:100})
    if err != nil { t.Fatal(err) }
    first, err := e.svc.Storage.SplitSamples(e.ctx, "tester", service.SplitSamplesInput{BatchID:b.ID, Quantities:[]int64{20}})
    if err != nil || len(first) != 1 { t.Fatalf("first split: %+v %v", first, err) }
    d, err := e.svc.Destruction.Create(e.ctx, "tester", service.CreateDestructionInput{BatchID:b.ID, Qty:20, Reason:"失活"})
    if err != nil { t.Fatal(err) }
    if _, err := e.svc.Destruction.Approve(e.ctx, "tester", d.ID, d.Version); err != nil { t.Fatal(err) }
    second, err := e.svc.Storage.SplitSamples(e.ctx, "tester", service.SplitSamplesInput{BatchID:b.ID, Quantities:[]int64{20}})
    if err != nil { t.Fatalf("resplit after destruction failed: %v", err) }
    if len(second) != 1 || second[0].SampleNo != "B22-B-S0002" { t.Fatalf("sample history was reused: %+v", second) }
}

func TestAnnotationControlAnnotationBug22(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
