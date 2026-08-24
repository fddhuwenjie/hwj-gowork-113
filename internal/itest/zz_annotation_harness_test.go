package itest

import (
    "testing"
    "germplasm/internal/service"
)

func TestAnnotationBug30(t *testing.T) {
    e := newTestEnv(t)
    r, err := e.svc.Resources.CreateResource(e.ctx, "creator", service.CreateResourceInput{Code:"B30-R", Name:"审计资源", Species:"Oryza sativa"})
    if err != nil { t.Fatal(err) }
    if _, err := e.svc.Resources.ArchiveResource(e.ctx, "archiver", r.ID, r.Version); err != nil { t.Fatal(err) }
    entries, _, err := e.svc.Audit.List(e.ctx, e.db, "resource", r.ID, "", 20)
    if err != nil { t.Fatal(err) }
    for _, entry := range entries {
        if entry.Action == "resource.archive" && entry.EntityID == r.ID && entry.EntityType == "resource" { return }
    }
    t.Fatalf("archive audit not attributable to resource %s: %+v", r.ID, entries)
}

func TestAnnotationControlAnnotationBug30(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
