package itest

import (
    "encoding/json"
    "fmt"
    "net"
    "net/http"
    "testing"
    "time"
    "germplasm/internal/domain"
    "germplasm/internal/httpx"
    "germplasm/internal/service"
)

func TestAnnotationBug27(t *testing.T) {
    e := newTestEnv(t)
    plan := e.breedToTest(t, "B27-C")
    test, err := e.svc.Purity.CreateTest(e.ctx, "tester", service.CreateTestInput{PlanID:plan.ID, SampleQty:100, CoverageRatio:1, PurityRate:1})
    if err != nil { t.Fatal(err) }
    if _, err := e.svc.Purity.SealTest(e.ctx, "tester", test.ID, test.Version); err != nil { t.Fatal(err) }
    rb, err := e.svc.Restock.Create(e.ctx, "tester", service.CreateRestockInput{RequestNo:"B27-R", PlanID:plan.ID, Qty:300})
    if err != nil { t.Fatal(err) }
    accepted, err := e.svc.Restock.Accept(e.ctx, "tester", rb.ID, rb.Version)
    if err != nil { t.Fatal(err) }
    listener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil { t.Fatal(err) }
    port := listener.Addr().(*net.TCPAddr).Port
    if err := listener.Close(); err != nil { t.Fatal(err) }
    server := httpx.NewServer(port, e.svc, e.db, e.clk, testLogger())
    errCh := make(chan error, 1)
    server.Start(errCh)
    t.Cleanup(func() { _ = server.Shutdown(e.ctx) })
    endpoint := fmt.Sprintf("http://127.0.0.1:%d/api/v1/restock-batches/%s", port, rb.ID)
    var response *http.Response
    for i := 0; i < 50; i++ {
        request, requestErr := http.NewRequestWithContext(e.ctx, http.MethodGet, endpoint, nil)
        if requestErr != nil { t.Fatal(requestErr) }
        response, err = http.DefaultClient.Do(request)
        if err == nil { break }
        select {
        case serverErr := <-errCh:
            t.Fatalf("HTTP server failed: %v", serverErr)
        default:
        }
        time.Sleep(10 * time.Millisecond)
    }
    if err != nil { t.Fatalf("GET restock: %v", err) }
    defer response.Body.Close()
    if response.StatusCode != http.StatusOK { t.Fatalf("GET restock status: %d", response.StatusCode) }
    var stored domain.RestockBatch
    if err := json.NewDecoder(response.Body).Decode(&stored); err != nil { t.Fatal(err) }
    if accepted.NewBatchID == "" || stored.NewBatchID != accepted.NewBatchID {
        t.Fatalf("accepted restock link diverged: response=%q stored=%q", accepted.NewBatchID, stored.NewBatchID)
    }
}

func TestAnnotationControlAnnotationBug27(t *testing.T) {
	if t.Name() == "" {
		t.Fatal("testing runtime did not provide a test name")
	}
}
