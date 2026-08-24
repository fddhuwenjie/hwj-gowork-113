package itest

import (
	"testing"
	"time"

	"germplasm/internal/domain"
	"germplasm/internal/service"
)

// TestFullChain 覆盖完整业务链：资源登记 -> 样本分装 -> 库位分配 -> 环境监测
// -> 保存规则启用 -> 出库审批 -> 繁育批次建立 -> 采样检测 -> 质量判定
// -> 回存验收 -> 谱系关联 -> 批次关闭，并校验历史快照。
func TestFullChain(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	chamber := "C01"
	rule := e.setupRule()
	res, acc, batch, _ := e.setupBase(1000, 400, chamber)
	e.setupSensors(chamber, 2)

	// 出库申请
	o, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo:      e.unique("OUT"),
		AccessionID:    acc.ID,
		BatchID:        batch.ID,
		Qty:            500,
		Purpose:        "繁育复壮",
		BreedingTarget: "获得 2000 粒合格种子",
		RuleVersionID:  rule.ID,
		Deadline:       e.clk.Now().Add(72 * time.Hour).Format(time.RFC3339Nano),
		IdempotencyKey: e.unique("idem"),
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	// 审批：冻结样本/库位/规则/繁育目标
	approved, err := e.svc.Outbound.Approve(ctx, "tester", o.ID, o.Version)
	if err != nil {
		t.Fatalf("出库审批失败: %v", err)
	}
	if approved.Status != domain.OutboundApproved {
		t.Fatalf("审批后状态应为 APPROVED，实际 %s", approved.Status)
	}
	// 校验冻结数量
	midBatch, err := e.svc.Storage.GetBatch(ctx, batch.ID)
	if err != nil {
		t.Fatalf("查询批次失败: %v", err)
	}
	if midBatch.QtyAvailable != 500 || midBatch.QtyFrozen != 500 {
		t.Fatalf("审批后批次数量错误: available=%d frozen=%d", midBatch.QtyAvailable, midBatch.QtyFrozen)
	}
	freezes, err := e.svc.Outbound.ListFreezes(ctx, o.ID)
	if err != nil || len(freezes) == 0 {
		t.Fatalf("冻结明细不能为空: %v", err)
	}
	var freezeSum int64
	for _, f := range freezes {
		freezeSum += f.Qty
		if f.LocationID == "" {
			t.Fatalf("冻结明细必须冻结库位")
		}
	}
	if freezeSum != 500 {
		t.Fatalf("冻结总量应为 500，实际 %d", freezeSum)
	}
	// 推进 1 小时并补充环境读数后出库
	e.clk.Advance(time.Hour)
	e.setupSensors(chamber, 1)
	fulfilled, err := e.svc.Outbound.Fulfill(ctx, "tester", o.ID, approved.Version)
	if err != nil {
		t.Fatalf("出库失败: %v", err)
	}
	if fulfilled.Status != domain.OutboundFulfilled {
		t.Fatalf("出库后状态应为 FULFILLED，实际 %s", fulfilled.Status)
	}
	afterBatch, _ := e.svc.Storage.GetBatch(ctx, batch.ID)
	if afterBatch.QtyFrozen != 0 || afterBatch.QtyOutbound != 500 || afterBatch.CheckConservation() != 0 {
		t.Fatalf("出库后批次数量不守恒: %+v", afterBatch)
	}

	// 繁育计划
	plan, err := e.svc.Breeding.CreatePlan(ctx, "tester", service.CreatePlanInput{
		PlanNo:            e.unique("PLN"),
		OutboundRequestID: o.ID,
		TargetQty:         2000,
		Plot:              "田间-A1",
		Deadline:          e.clk.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("建立繁育计划失败: %v", err)
	}
	// 田间观察
	for i, rate := range []float64{0.92, 0.90, 0.91} {
		_, err := e.svc.Breeding.AddObservation(ctx, "tester", plan.ID, service.AddObservationInput{
			ObservedAt:      e.clk.Now().Add(time.Duration(i+1) * 24 * time.Hour).Format(time.RFC3339Nano),
			GerminationRate: rate,
			Vigor:           "强",
		})
		if err != nil {
			t.Fatalf("追加田间观察失败: %v", err)
		}
	}
	// 纯度检测并封存
	test, err := e.svc.Purity.CreateTest(ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 400, CoverageRatio: 0.98, PurityRate: 0.97,
		IdempotencyKey: e.unique("idem"),
	})
	if err != nil {
		t.Fatalf("登记纯度检测失败: %v", err)
	}
	sealed, err := e.svc.Purity.SealTest(ctx, "tester", test.ID, test.Version)
	if err != nil {
		t.Fatalf("封存检测失败: %v", err)
	}
	if sealed.Verdict != domain.VerdictPass {
		t.Fatalf("检测结论应为 PASS，实际 %s", sealed.Verdict)
	}
	// 回存验收
	rb, err := e.svc.Restock.Create(ctx, "tester", service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 1800, IdempotencyKey: e.unique("idem"),
	})
	if err != nil {
		t.Fatalf("创建回存验收单失败: %v", err)
	}
	accepted, err := e.svc.Restock.Accept(ctx, "tester", rb.ID, rb.Version)
	if err != nil {
		t.Fatalf("回存验收失败: %v", err)
	}
	if accepted.Status != domain.RestockAccepted || accepted.NewBatchID == "" {
		t.Fatalf("回存验收结果错误: %+v", accepted)
	}
	// 新批次与谱系
	newBatch, err := e.svc.Storage.GetBatch(ctx, accepted.NewBatchID)
	if err != nil {
		t.Fatalf("查询新批次失败: %v", err)
	}
	if newBatch.Kind != domain.BatchRestock || newBatch.MotherBatchID != batch.ID || newBatch.QtyTotal != 1800 {
		t.Fatalf("回存新批次属性错误: %+v", newBatch)
	}
	lineage, err := e.svc.Lineage.GetLineage(ctx, newBatch.ID)
	if err != nil {
		t.Fatalf("查询谱系失败: %v", err)
	}
	if len(lineage.Parents) != 1 || lineage.Parents[0].ParentBatchID != batch.ID {
		t.Fatalf("谱系关联错误: %+v", lineage)
	}
	// 繁育计划已完成
	finalPlan, _ := e.svc.Breeding.GetPlan(ctx, plan.ID)
	if finalPlan.Status != domain.PlanCompleted {
		t.Fatalf("回存验收后计划应为 COMPLETED，实际 %s", finalPlan.Status)
	}
	// 历史快照：出库审批/出库/计划建立/检测封存/回存验收
	snapTypes := map[string]int{}
	for _, pair := range [][2]string{
		{"outbound", o.ID}, {"breeding_plan", plan.ID}, {"purity_test", test.ID}, {"restock", rb.ID},
	} {
		page, err := e.svc.Repos.Snapshots.List(ctx, e.db, pair[0], pair[1], "", 50)
		if err != nil {
			t.Fatalf("查询快照失败: %v", err)
		}
		for _, s := range page.Items {
			snapTypes[s.Event]++
		}
	}
	for _, event := range []string{"APPROVED", "FULFILLED", "CREATED", "SEALED", "ACCEPTED"} {
		if snapTypes[event] == 0 {
			t.Fatalf("缺少历史快照事件 %s，当前快照: %v", event, snapTypes)
		}
	}
	// 资源与 accession 状态
	if res.Status != domain.ResourceActive {
		t.Fatalf("资源状态异常: %s", res.Status)
	}
	finalAcc, _ := e.svc.Resources.GetAccession(ctx, acc.ID)
	if finalAcc.Status != domain.AccessionInStock {
		t.Fatalf("accession 状态应为 IN_STOCK，实际 %s", finalAcc.Status)
	}
}
