package itest

import (
	"encoding/json"
	"testing"

	"germplasm/internal/service"
)

func TestAnnotationBug09(t *testing.T) {
	e := newTestEnv(t)
	plan := e.breedToTest(t, "A09")
	pt, err := e.svc.Purity.CreateTest(e.ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 100, CoverageRatio: 1, PurityRate: 0.99,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.svc.Purity.SealTest(e.ctx, "tester", pt.ID, pt.Version); err != nil {
		t.Fatal(err)
	}
	rb, err := e.svc.Restock.Create(e.ctx, "tester", service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.svc.Restock.Accept(e.ctx, "tester", rb.ID, rb.Version); err != nil {
		t.Fatal(err)
	}
	page, err := e.svc.Repos.Snapshots.List(e.ctx, e.db, "restock", rb.ID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, snap := range page.Items {
		if snap.Event != "ACCEPTED" {
			continue
		}
		var payload struct {
			MotherBatch struct {
				ID string `json:"id"`
			} `json:"mother_batch"`
		}
		if err := json.Unmarshal([]byte(snap.Payload), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.MotherBatch.ID != plan.BatchID {
			t.Fatalf("snapshot mother=%q want=%q payload=%s", payload.MotherBatch.ID, plan.BatchID, snap.Payload)
		}
		return
	}
	t.Fatal("missing ACCEPTED snapshot")
}

func TestAnnotationControlAnnotationBug09(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
