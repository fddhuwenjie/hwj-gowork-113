package itest

import (
	"testing"
	"time"

	"germplasm/internal/service"
)

// TestOutboundIdempotency 出库申请重复提交凭幂等键返回首个申请；
// 同键不同请求体必须报幂等冲突。
func TestOutboundIdempotency(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(500, 250, "C11")
	key := e.unique("idem")
	in := service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 100,
		RuleVersionID: rule.ID, Deadline: e.clk.Now().Add(24 * time.Hour).Format(time.RFC3339Nano),
		IdempotencyKey: key,
	}
	first, err := e.svc.Outbound.Create(ctx, "tester", in)
	if err != nil {
		t.Fatalf("首次创建失败: %v", err)
	}
	second, err := e.svc.Outbound.Create(ctx, "tester", in)
	if err != nil {
		t.Fatalf("幂等重放应成功: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("幂等重放应返回同一申请: %s vs %s", first.ID, second.ID)
	}
	// 同键不同请求体
	in.Qty = 200
	_, err = e.svc.Outbound.Create(ctx, "tester", in)
	mustErrCode(t, err, "IDEMPOTENCY_CONFLICT")
	// 全局只创建了一个申请。
	page, _ := e.svc.Outbound.List(ctx, "", "", 50)
	if len(page.Items) != 1 {
		t.Fatalf("幂等约束下应只有 1 条申请，实际 %d", len(page.Items))
	}
}

// TestPurityIdempotency 检测登记幂等重放。
func TestPurityIdempotency(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C12")
	key := e.unique("idem")
	in := service.CreateTestInput{PlanID: plan.ID, SampleQty: 100, CoverageRatio: 1.0, PurityRate: 0.98, IdempotencyKey: key}
	first, err := e.svc.Purity.CreateTest(ctx, "tester", in)
	if err != nil {
		t.Fatalf("首次登记失败: %v", err)
	}
	second, err := e.svc.Purity.CreateTest(ctx, "tester", in)
	if err != nil {
		t.Fatalf("幂等重放应成功: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("幂等重放应返回同一检测")
	}
	page, _ := e.svc.Purity.ListTests(ctx, plan.ID, "", 50)
	if len(page.Items) != 1 {
		t.Fatalf("应只有 1 条检测，实际 %d", len(page.Items))
	}
}

// TestRestockIdempotency 回存单幂等重放。
func TestRestockIdempotency(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C13")
	key := e.unique("idem")
	in := service.CreateRestockInput{RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 800, IdempotencyKey: key}
	first, err := e.svc.Restock.Create(ctx, "tester", in)
	if err != nil {
		t.Fatalf("首次创建失败: %v", err)
	}
	second, err := e.svc.Restock.Create(ctx, "tester", in)
	if err != nil {
		t.Fatalf("幂等重放应成功: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("幂等重放应返回同一回存单")
	}
}
