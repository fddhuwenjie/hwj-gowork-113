package itest

import (
	"testing"
	"time"

	"germplasm/internal/domain"
	"germplasm/internal/service"
)

// TestLineageVersionChain 谱系版本链：回存批次记录母批，旧检测只读，
// 多次繁育回存形成多代谱系。
func TestLineageVersionChain(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C20")
	test, err := e.svc.Purity.CreateTest(ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 300, CoverageRatio: 1.0, PurityRate: 0.99,
	})
	if err != nil {
		t.Fatalf("登记检测失败: %v", err)
	}
	if _, err := e.svc.Purity.SealTest(ctx, "tester", test.ID, test.Version); err != nil {
		t.Fatalf("封存失败: %v", err)
	}
	rb, err := e.svc.Restock.Create(ctx, "tester", service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 900,
	})
	if err != nil {
		t.Fatalf("创建回存单失败: %v", err)
	}
	accepted, err := e.svc.Restock.Accept(ctx, "tester", rb.ID, rb.Version)
	if err != nil {
		t.Fatalf("回存验收失败: %v", err)
	}
	planAfter, _ := e.svc.Breeding.GetPlan(ctx, plan.ID)
	motherID := planAfter.BatchID
	// 谱系：新批次的母批是原批次
	view, err := e.svc.Lineage.GetLineage(ctx, accepted.NewBatchID)
	if err != nil {
		t.Fatalf("查询谱系失败: %v", err)
	}
	if len(view.Parents) != 1 || view.Parents[0].ParentBatchID != motherID || view.Parents[0].Relation != "RESTOCK" {
		t.Fatalf("谱系边错误: %+v", view.Parents)
	}
	// 母批视图能看到子批
	motherView, err := e.svc.Lineage.GetLineage(ctx, motherID)
	if err != nil {
		t.Fatalf("查询母批谱系失败: %v", err)
	}
	if len(motherView.Children) != 1 || motherView.Children[0].ChildBatchID != accepted.NewBatchID {
		t.Fatalf("母批子代谱系错误: %+v", motherView.Children)
	}
	// 旧检测只读：封存后版本递增，再次封存被拒绝
	sealed, _ := e.svc.Purity.GetTest(ctx, test.ID)
	if !sealed.Sealed || sealed.SealedAt == nil {
		t.Fatalf("检测应为已封存只读")
	}
	_, err = e.svc.Purity.SealTest(ctx, "tester", test.ID, sealed.Version)
	mustErrCode(t, err, "STATE_CONFLICT")
	// 全库谱系无异常
	anomalies, err := e.svc.Lineage.Anomalies(ctx)
	if err != nil {
		t.Fatalf("谱系异常检测失败: %v", err)
	}
	if len(anomalies) != 0 {
		t.Fatalf("正常谱系不应有异常: %+v", anomalies)
	}
}

// TestLineageAnomalyDetection 谱系异常检测：自环、成环与孤儿批次。
func TestLineageAnomalyDetection(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	res, _ := e.svc.Resources.CreateResource(ctx, "tester", service.CreateResourceInput{
		Code: e.unique("RES"), Name: "种质", Species: "S", Category: "C",
	})
	acc, _ := e.svc.Resources.CreateAccession(ctx, "tester", service.CreateAccessionInput{
		ResourceID: res.ID, AccessionNo: e.unique("ACC"),
	})
	b1, _ := e.svc.Storage.CreateOriginalBatch(ctx, "tester", service.CreateBatchInput{
		AccessionID: acc.ID, BatchNo: e.unique("BAT"), QtyTotal: 10,
	})
	b2, _ := e.svc.Storage.CreateOriginalBatch(ctx, "tester", service.CreateBatchInput{
		AccessionID: acc.ID, BatchNo: e.unique("BAT"), QtyTotal: 10,
	})
	now := e.clk.Now()
	// 人为构造环：b1->b2, b2->b1
	for _, pair := range [][2]string{{b1.ID, b2.ID}, {b2.ID, b1.ID}} {
		err := e.svc.Repos.Lineage.InsertEdge(ctx, e.db, &domain.LineageEdge{
			ID: domain.NewID(domain.PrefixLineage), ResourceID: res.ID,
			ParentBatchID: pair[0], ChildBatchID: pair[1], Relation: "RESTOCK", CreatedAt: now,
		})
		if err != nil {
			t.Fatalf("插入谱系边失败: %v", err)
		}
	}
	anomalies, err := e.svc.Lineage.Anomalies(ctx)
	if err != nil {
		t.Fatalf("谱系异常检测失败: %v", err)
	}
	types := map[string]bool{}
	for _, a := range anomalies {
		types[a.Type] = true
	}
	if !types["CYCLE"] {
		t.Fatalf("应检测出谱系环: %+v", anomalies)
	}
}

// TestGerminationDecline 连续发芽率下降巡检。
func TestGerminationDecline(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C21")
	rates := []float64{0.95, 0.93, 0.90, 0.88}
	for i, rate := range rates {
		_, err := e.svc.Breeding.AddObservation(ctx, "tester", plan.ID, service.AddObservationInput{
			ObservedAt:      e.clk.Now().Add(time.Duration(i+1) * time.Hour).Format(time.RFC3339Nano),
			GerminationRate: rate,
		})
		if err != nil {
			t.Fatalf("追加观察失败: %v", err)
		}
	}
	items, err := e.svc.Risk.GerminationDeclines(ctx)
	if err != nil {
		t.Fatalf("发芽率下降巡检失败: %v", err)
	}
	found := false
	for _, item := range items {
		if item.PlanID == plan.ID && item.ConsecutiveDrops >= 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("应识别连续发芽率下降: %+v", items)
	}
}

// TestBatchClosedWhenMotherExhausted 母批耗尽时回存验收关闭母批。
func TestBatchClosedWhenMotherExhausted(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C22") // 出库 200，母批 500 剩 300
	planAfter, _ := e.svc.Breeding.GetPlan(ctx, plan.ID)
	motherID := planAfter.BatchID
	// 把母批剩余 300 全部销毁，使其耗尽。
	d, err := e.svc.Destruction.Create(ctx, "tester", service.CreateDestructionInput{
		BatchID: motherID, Qty: 300, Reason: "活力丧失",
	})
	if err != nil {
		t.Fatalf("创建销毁申请失败: %v", err)
	}
	if _, err := e.svc.Destruction.Approve(ctx, "tester", d.ID, d.Version); err != nil {
		t.Fatalf("批准销毁失败: %v", err)
	}
	test, _ := e.svc.Purity.CreateTest(ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 300, CoverageRatio: 1.0, PurityRate: 0.99,
	})
	if _, err := e.svc.Purity.SealTest(ctx, "tester", test.ID, test.Version); err != nil {
		t.Fatalf("封存失败: %v", err)
	}
	rb, _ := e.svc.Restock.Create(ctx, "tester", service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 900,
	})
	if _, err := e.svc.Restock.Accept(ctx, "tester", rb.ID, rb.Version); err != nil {
		t.Fatalf("回存验收失败: %v", err)
	}
	mother, _ := e.svc.Storage.GetBatch(ctx, motherID)
	if mother.Status != domain.BatchClosed || mother.ClosedAt == nil {
		t.Fatalf("耗尽母批应被关闭: %+v", mother)
	}
}
