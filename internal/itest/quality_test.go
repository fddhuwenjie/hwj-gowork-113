package itest

import (
	"testing"
	"time"

	"germplasm/internal/domain"
	"germplasm/internal/service"
)

// breedToTest 完成出库与繁育计划建立，返回可登记检测的计划。
func (e *testEnv) breedToTest(t *testing.T, chamber string) *domain.BreedingPlan {
	t.Helper()
	ctx := e.ctx
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
	plan, err := e.svc.Breeding.CreatePlan(ctx, "tester", service.CreatePlanInput{
		PlanNo: e.unique("PLN"), OutboundRequestID: o.ID, TargetQty: 1000,
		Deadline: e.clk.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("建立繁育计划失败: %v", err)
	}
	return plan
}

// TestRestockBlockedByFailVerdict 纯度不合格时不得回存为合格批次。
func TestRestockBlockedByFailVerdict(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C07")
	test, err := e.svc.Purity.CreateTest(ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 300, CoverageRatio: 0.99, PurityRate: 0.90, // 低于 0.95 门槛
	})
	if err != nil {
		t.Fatalf("登记检测失败: %v", err)
	}
	sealed, err := e.svc.Purity.SealTest(ctx, "tester", test.ID, test.Version)
	if err != nil {
		t.Fatalf("封存失败: %v", err)
	}
	if sealed.Verdict != domain.VerdictFail {
		t.Fatalf("纯度 0.90 应判定 FAIL，实际 %s", sealed.Verdict)
	}
	rb, err := e.svc.Restock.Create(ctx, "tester", service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 800,
	})
	if err != nil {
		t.Fatalf("创建回存单失败: %v", err)
	}
	_, err = e.svc.Restock.Accept(ctx, "tester", rb.ID, rb.Version)
	mustErrCode(t, err, "QUALITY_VIOLATION")
	// 事务回滚：不得产生新批次，计划仍为 ACTIVE。
	p, _ := e.svc.Breeding.GetPlan(ctx, plan.ID)
	if p.Status != domain.PlanActive {
		t.Fatalf("验收失败后计划应仍为 ACTIVE，实际 %s", p.Status)
	}
	page, _ := e.svc.Storage.ListBatches(ctx, "", string(domain.BatchRestock), "", 50)
	if len(page.Items) != 0 {
		t.Fatalf("验收失败不得创建回存批次，实际 %d 个", len(page.Items))
	}
}

// TestRestockBlockedByCoverage 检测覆盖不足时不得回存。
func TestRestockBlockedByCoverage(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C08")
	test, err := e.svc.Purity.CreateTest(ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 300, CoverageRatio: 0.60, PurityRate: 0.99, // 覆盖率低于 1.0 门槛
	})
	if err != nil {
		t.Fatalf("登记检测失败: %v", err)
	}
	sealed, err := e.svc.Purity.SealTest(ctx, "tester", test.ID, test.Version)
	if err != nil {
		t.Fatalf("封存失败: %v", err)
	}
	if sealed.Verdict != domain.VerdictFail {
		t.Fatalf("覆盖率不足应判定 FAIL，实际 %s", sealed.Verdict)
	}
	rb, _ := e.svc.Restock.Create(ctx, "tester", service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 800,
	})
	_, err = e.svc.Restock.Accept(ctx, "tester", rb.ID, rb.Version)
	mustErrCode(t, err, "QUALITY_VIOLATION")
}

// TestRestockBlockedWithoutSealedTest 无封存结论时不得回存。
func TestRestockBlockedWithoutSealedTest(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C09")
	rb, err := e.svc.Restock.Create(ctx, "tester", service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 800,
	})
	if err != nil {
		t.Fatalf("创建回存单失败: %v", err)
	}
	_, err = e.svc.Restock.Accept(ctx, "tester", rb.ID, rb.Version)
	mustErrCode(t, err, "QUALITY_VIOLATION")
}

// TestRestockBlockedByTimeout 繁育计划被超时作业标记为 TIMEOUT 后，
// 超时终态应拒绝新的回存链路：不得创建回存单（拒绝进入待验收），更不得
// 借验收把计划推进为 COMPLETED。回存编排与计划状态机之间不允许越权转换。
func TestRestockBlockedByTimeout(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C20")
	// 提前登记并封存合格结论，确保后续回存失败仅由 TIMEOUT 状态导致，
	// 而非缺少封存结论或质量门槛不达标。
	test, err := e.svc.Purity.CreateTest(ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 300, CoverageRatio: 1.0, PurityRate: 0.98,
	})
	if err != nil {
		t.Fatalf("登记检测失败: %v", err)
	}
	if _, err := e.svc.Purity.SealTest(ctx, "tester", test.ID, test.Version); err != nil {
		t.Fatalf("封存失败: %v", err)
	}
	// 在计划仍为 ACTIVE 时创建回存单（进入待验收），随后让超时巡检将其置为 TIMEOUT，
	// 模拟“超时前已存在待验收回存单”这一越权场景的入口。
	rb, err := e.svc.Restock.Create(ctx, "tester", service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 800,
	})
	if err != nil {
		t.Fatalf("超时前创建回存单不应失败: %v", err)
	}
	// 推进时间超过繁育期限，超时巡检将 ACTIVE 计划标记为 TIMEOUT。
	e.clk.Advance(31 * 24 * time.Hour)
	e.sched.RunOnceForTest(ctx)
	p, err := e.svc.Breeding.GetPlan(ctx, plan.ID)
	if err != nil {
		t.Fatalf("查询计划失败: %v", err)
	}
	if p.Status != domain.PlanTimeout {
		t.Fatalf("超期计划应标记 TIMEOUT，实际 %s", p.Status)
	}
	// 越权链路 1：超时后不得验收既有回存单，也不得把计划推进为 COMPLETED。
	_, err = e.svc.Restock.Accept(ctx, "tester", rb.ID, rb.Version)
	mustErrCode(t, err, "STATE_CONFLICT")
	finalPlan, _ := e.svc.Breeding.GetPlan(ctx, plan.ID)
	if finalPlan.Status != domain.PlanTimeout {
		t.Fatalf("超时计划不得被验收推进为 COMPLETED，实际 %s", finalPlan.Status)
	}
	batches, _ := e.svc.Storage.ListBatches(ctx, "", string(domain.BatchRestock), "", 50)
	if len(batches.Items) != 0 {
		t.Fatalf("超时计划验收失败不得创建回存批次，实际 %d 个", len(batches.Items))
	}
	// 越权链路 2：超时后不得新建回存验收单（拒绝进入待验收）。
	_, err = e.svc.Restock.Create(ctx, "tester", service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 800,
	})
	mustErrCode(t, err, "STATE_CONFLICT")
	page, _ := e.svc.Restock.List(ctx, "", "", 50)
	if len(page.Items) != 1 { // 仅含超时前创建的那一张
		t.Fatalf("超时计划不得新增回存单，期望 1 张，实际 %d 张", len(page.Items))
	}
}

// TestLateTestCannotOverride 迟到检测不得覆盖当前质量结论：
// 已封存后，早于封存时刻的检测只能只读登记，封存第二次一律拒绝。
func TestLateTestCannotOverride(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C10")
	pass, err := e.svc.Purity.CreateTest(ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 300, CoverageRatio: 1.0, PurityRate: 0.98,
		TestedAt: e.clk.Now().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("登记检测失败: %v", err)
	}
	if _, err := e.svc.Purity.SealTest(ctx, "tester", pass.ID, pass.Version); err != nil {
		t.Fatalf("封存失败: %v", err)
	}
	// 迟到检测：tested_at 早于封存时刻，结果不合格。
	late, err := e.svc.Purity.CreateTest(ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 300, CoverageRatio: 1.0, PurityRate: 0.50,
		TestedAt: e.clk.Now().Add(-time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("迟到检测登记失败: %v", err)
	}
	isLate, err := e.svc.Purity.IsLateTest(ctx, late)
	if err != nil {
		t.Fatalf("判断迟到检测失败: %v", err)
	}
	if !isLate {
		t.Fatalf("应识别为迟到检测")
	}
	// 迟到检测不得封存覆盖结论。
	_, err = e.svc.Purity.SealTest(ctx, "tester", late.ID, late.Version)
	mustErrCode(t, err, "STATE_CONFLICT")
	// 当前结论仍为 PASS。
	sealed, err := e.svc.Repos.Purity.LatestSealed(ctx, e.db, plan.ID)
	if err != nil || sealed == nil || sealed.Verdict != domain.VerdictPass {
		t.Fatalf("当前质量结论应仍为 PASS: %+v err=%v", sealed, err)
	}
}
