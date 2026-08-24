package itest

import (
	"testing"
	"time"

	"germplasm/internal/service"
)

// TestOutboundIdempotency_DifferentDeadlineIsConflict 同一幂等键但交付截止时间
// 变化时，属于不同请求语义，必须报幂等冲突而非误返回首单。
func TestOutboundIdempotency_DifferentDeadlineIsConflict(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(500, 250, "C21")
	key := e.unique("idem")
	base := service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 100,
		RuleVersionID: rule.ID, IdempotencyKey: key,
	}

	// 首单：24 小时后交付
	in24 := base
	in24.Deadline = e.clk.Now().Add(24 * time.Hour).Format(time.RFC3339Nano)
	first, err := e.svc.Outbound.Create(ctx, "tester", in24)
	if err != nil {
		t.Fatalf("首次创建失败: %v", err)
	}

	// 同键、同其余字段，仅交付截止时间变化（72 小时后）
	in72 := base
	in72.Deadline = e.clk.Now().Add(72 * time.Hour).Format(time.RFC3339Nano)
	second, err := e.svc.Outbound.Create(ctx, "tester", in72)
	if err == nil {
		t.Fatalf("截止时间变化属于不同请求语义，应报幂等冲突，却返回了首单 %s == %s", second.ID, first.ID)
	}
	mustErrCode(t, err, "IDEMPOTENCY_CONFLICT")

	// 全局仍只有首单。
	page, _ := e.svc.Outbound.List(ctx, "", "", 50)
	if len(page.Items) != 1 {
		t.Fatalf("应只保留首单，实际 %d 条", len(page.Items))
	}
}

// TestIdempotencyKeyIsolatesByEndpoint 同一幂等键被不同创建业务复用时必须互相隔离：
// 出库申请创建后，该键用于纯度检测登记不应被判为幂等冲突。
func TestIdempotencyKeyIsolatesByEndpoint(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(500, 250, "C22")
	e.setupSensors("C22", 2)

	key := e.unique("shared")
	// 用该键创建出库申请。
	out, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 100,
		RuleVersionID: rule.ID, Deadline: e.clk.Now().Add(24 * time.Hour).Format(time.RFC3339Nano),
		IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	// 完成出库并建立繁育计划，为纯度检测做准备。
	if _, err := e.svc.Outbound.Approve(ctx, "tester", out.ID, out.Version); err != nil {
		t.Fatalf("审批失败: %v", err)
	}
	cur, _ := e.svc.Outbound.Get(ctx, out.ID)
	if _, err := e.svc.Outbound.Fulfill(ctx, "tester", out.ID, cur.Version); err != nil {
		t.Fatalf("出库失败: %v", err)
	}
	plan, err := e.svc.Breeding.CreatePlan(ctx, "tester", service.CreatePlanInput{
		PlanNo: e.unique("PLN"), OutboundRequestID: out.ID, TargetQty: 1000,
		Deadline: e.clk.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("建立繁育计划失败: %v", err)
	}
	// 同一键复用于纯度检测登记，必须成功而非误报幂等冲突。
	test, err := e.svc.Purity.CreateTest(ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 100, CoverageRatio: 1.0, PurityRate: 0.98, IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("不同创建业务应互相隔离，复用同一幂等键登记检测应成功: %v", err)
	}
	if test.ID == "" {
		t.Fatal("检测登记未返回有效记录")
	}
}
