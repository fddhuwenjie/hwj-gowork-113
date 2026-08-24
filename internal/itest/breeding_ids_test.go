package itest

import (
	"testing"
	"time"

	"germplasm/internal/domain"
	"germplasm/internal/service"
)

// TestBreedingPlanKeepsIndependentSourceIDs 地块使用大区/小区分层编码（含 "/"）时，
// 繁育计划详情读取不得串写母批 ID 与原出库申请 ID，二者须保持独立，
// 使回存验收可分别追溯母批来源与出库申请来源。
func TestBreedingPlanKeepsIndependentSourceIDs(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	chamber := "C30"
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(500, 250, chamber)
	e.setupSensors(chamber, 2)

	o, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 200,
		RuleVersionID: rule.ID, BreedingTarget: "目标", Deadline: e.clk.Now().Add(72 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	if _, err := e.svc.Outbound.Approve(ctx, "tester", o.ID, o.Version); err != nil {
		t.Fatalf("审批失败: %v", err)
	}
	cur, _ := e.svc.Outbound.Get(ctx, o.ID)
	if _, err := e.svc.Outbound.Fulfill(ctx, "tester", o.ID, cur.Version); err != nil {
		t.Fatalf("出库失败: %v", err)
	}

	// 大区/小区分层编码：plot 含 "/"，曾触发详情读取串写。
	plan, err := e.svc.Breeding.CreatePlan(ctx, "tester", service.CreatePlanInput{
		PlanNo:            e.unique("PLN"),
		OutboundRequestID: o.ID,
		TargetQty:         1000,
		Plot:              "大区A/小区2",
		Deadline:          e.clk.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("建立繁育计划失败: %v", err)
	}
	if plan.Plot != "大区A/小区2" {
		t.Fatalf("plot 编码未被保留: %q", plan.Plot)
	}

	// 详情读取后两个来源标识须各自独立、互不相同。
	got, err := e.svc.Breeding.GetPlan(ctx, plan.ID)
	if err != nil {
		t.Fatalf("查询繁育计划失败: %v", err)
	}
	if got.OutboundRequestID != o.ID {
		t.Fatalf("原出库申请标识被串写：期望 %s，实际 %s", o.ID, got.OutboundRequestID)
	}
	if got.BatchID != batch.ID {
		t.Fatalf("母批标识被串写：期望 %s，实际 %s", batch.ID, got.BatchID)
	}
	if got.OutboundRequestID == got.BatchID {
		t.Fatalf("母批标识与原出库申请标识被串写为同一值：%s", got.BatchID)
	}

	// 回存验收须能分别追溯母批（BatchID）与出库申请（OutboundRequestID）：
	// 验收事务内按 OutboundRequestID 取出库申请冻结规则，按 BatchID 取母批建谱系。
	test, err := e.svc.Purity.CreateTest(ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 200, CoverageRatio: 1.0, PurityRate: 0.99,
	})
	if err != nil {
		t.Fatalf("登记检测失败: %v", err)
	}
	if _, err := e.svc.Purity.SealTest(ctx, "tester", test.ID, test.Version); err != nil {
		t.Fatalf("封存失败: %v", err)
	}
	rb, err := e.svc.Restock.Create(ctx, "tester", service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 800,
	})
	if err != nil {
		t.Fatalf("创建回存单失败: %v", err)
	}
	accepted, err := e.svc.Restock.Accept(ctx, "tester", rb.ID, rb.Version)
	if err != nil {
		t.Fatalf("回存验收失败，无法追溯母批来源: %v", err)
	}
	if accepted.NewBatchID == "" {
		t.Fatalf("回存验收未创建新批次")
	}
	newBatch, err := e.svc.Storage.GetBatch(ctx, accepted.NewBatchID)
	if err != nil {
		t.Fatalf("查询新批次失败: %v", err)
	}
	if newBatch.MotherBatchID != batch.ID {
		t.Fatalf("回存批次母批关联错误：期望 %s，实际 %s", batch.ID, newBatch.MotherBatchID)
	}
	view, err := e.svc.Lineage.GetLineage(ctx, newBatch.ID)
	if err != nil {
		t.Fatalf("查询谱系失败: %v", err)
	}
	if len(view.Parents) != 1 || view.Parents[0].ParentBatchID != batch.ID {
		t.Fatalf("谱系母批来源错误: %+v", view.Parents)
	}
	// 计划完成联动。
	finalPlan, _ := e.svc.Breeding.GetPlan(ctx, plan.ID)
	if finalPlan.Status != domain.PlanCompleted {
		t.Fatalf("回存验收后计划应为 COMPLETED，实际 %s", finalPlan.Status)
	}
	if finalPlan.OutboundRequestID != o.ID || finalPlan.BatchID != batch.ID {
		t.Fatalf("计划完成后来源标识仍须独立：outbound=%s batch=%s", finalPlan.OutboundRequestID, finalPlan.BatchID)
	}
}
