package itest

import (
    "testing"
    "germplasm/internal/domain"
    "germplasm/internal/service"
)

func TestAnnotationBug13(t *testing.T) {
    e := newTestEnv(t)
    _, _, batch, _ := e.setupBase(500, 100, "B13-COLD")
    pending, err := e.svc.Destruction.Create(e.ctx, "tester", service.CreateDestructionInput{
        BatchID: batch.ID, Qty: 300, Reason: "复核盘亏销毁",
    })
    if err != nil { t.Fatal(err) }
    competing, err := e.svc.Destruction.Create(e.ctx, "tester", service.CreateDestructionInput{
        BatchID: batch.ID, Qty: 300, Reason: "活力丧失",
    })
    if err != nil { t.Fatal(err) }
    if _, err := e.svc.Destruction.Approve(e.ctx, "tester", competing.ID, competing.Version); err != nil { t.Fatal(err) }
    samples, err := e.svc.Repos.Samples.ListByBatch(e.ctx, e.db, batch.ID, domain.SampleInStock)
    if err != nil || len(samples) == 0 { t.Fatalf("samples=%+v err=%v", samples, err) }
    before := samples[0]
    if _, err := e.svc.Destruction.Approve(e.ctx, "tester", pending.ID, pending.Version); err == nil {
        t.Fatal("insufficient approval unexpectedly succeeded")
    }
    after, err := e.svc.Repos.Samples.Get(e.ctx, e.db, before.ID)
    if err != nil { t.Fatal(err) }
    if after.Qty != before.Qty || after.Status != before.Status || after.Version != before.Version {
        t.Fatalf("failed approval mutated sample: before=%+v after=%+v", before, after)
    }
}

func TestAnnotationControlAnnotationBug13(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
