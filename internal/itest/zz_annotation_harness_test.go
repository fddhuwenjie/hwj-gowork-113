package itest

import (
    "testing"
    "germplasm/internal/domain"
    "germplasm/internal/service"
)

func TestAnnotationBug17(t *testing.T) {
    e := newTestEnv(t)
    res, err := e.svc.Resources.CreateResource(e.ctx, "tester", service.CreateResourceInput{Code:e.unique("RES"), Name:"稻", Species:"S", Category:"C"})
    if err != nil { t.Fatal(err) }
    acc, err := e.svc.Resources.CreateAccession(e.ctx, "tester", service.CreateAccessionInput{ResourceID:res.ID, AccessionNo:e.unique("ACC")})
    if err != nil { t.Fatal(err) }
    batches := make([]*domain.Batch, 3)
    for i := range batches {
        batches[i], err = e.svc.Storage.CreateOriginalBatch(e.ctx, "tester", service.CreateBatchInput{AccessionID:acc.ID, BatchNo:e.unique("BAT"), QtyTotal:10})
        if err != nil { t.Fatal(err) }
    }
    now := e.clk.Now()
    pairs := [][2]string{{batches[0].ID,batches[1].ID},{batches[0].ID,batches[2].ID}}
    for _, pair := range pairs {
        if err := e.svc.Repos.Lineage.InsertEdge(e.ctx, e.db, &domain.LineageEdge{ID:domain.NewID(domain.PrefixLineage), ResourceID:res.ID, ParentBatchID:pair[0], ChildBatchID:pair[1], Relation:"RESTOCK", CreatedAt:now}); err != nil { t.Fatal(err) }
    }
    view, err := e.svc.Lineage.GetLineage(e.ctx, batches[1].ID)
    if err != nil { t.Fatal(err) }
    if len(view.Parents) != 1 || view.Parents[0].ChildBatchID != batches[1].ID || len(view.Children) != 0 { t.Fatalf("leaked lineage: %+v", view) }
}

func TestAnnotationControlAnnotationBug17(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
