package itest

import (
    "testing"
    "germplasm/internal/service"
)

func TestAnnotationBug21(t *testing.T) {
    e := newTestEnv(t)
    mkResource := func(code string) string {
        r, err := e.svc.Resources.CreateResource(e.ctx, "tester", service.CreateResourceInput{Code:code, Name:code, Species:"Oryza sativa"})
        if err != nil { t.Fatal(err) }
        return r.ID
    }
    target := mkResource("B21-TARGET")
    foreign := mkResource("B21-FOREIGN")
    for i, no := range []string{"B21-A1", "B21-A2", "B21-A3"} {
        if _, err := e.svc.Resources.CreateAccession(e.ctx, "tester", service.CreateAccessionInput{ResourceID:target, AccessionNo:no, Origin:"云南"}); err != nil { t.Fatalf("target %d: %v", i, err) }
    }
    if _, err := e.svc.Resources.CreateAccession(e.ctx, "tester", service.CreateAccessionInput{ResourceID:foreign, AccessionNo:"B21-X", Origin:"海南"}); err != nil { t.Fatal(err) }
    first, err := e.svc.Resources.ListAccessions(e.ctx, target, "", 2)
    if err != nil { t.Fatal(err) }
    if len(first.Items) != 2 || first.NextCursor == "" { t.Fatalf("first page=%+v", first) }
    second, err := e.svc.Resources.ListAccessions(e.ctx, target, first.NextCursor, 2)
    if err != nil { t.Fatal(err) }
    if len(second.Items) != 1 || second.Items[0].ResourceID != target || second.Items[0].AccessionNo != "B21-A3" {
        t.Fatalf("resource filter lost on next page: %+v", second.Items)
    }
}

func TestAnnotationControlAnnotationBug21(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
