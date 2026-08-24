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

// TestCrossEndpointIdempotencyIsolation 不同业务入口共用同一幂等键时，
// 各自的重复提交语义相互隔离：检测登记与回存申请即使 key 相同也互不
// 冲突、互不重放，各自独立返回首个实体。
func TestCrossEndpointIdempotencyIsolation(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C14")
	// 两个入口共用同一幂等键，请求体与业务对象完全不同。
	key := e.unique("shared")

	purityIn := service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 100, CoverageRatio: 1.0, PurityRate: 0.98,
		IdempotencyKey: key,
	}
	test, err := e.svc.Purity.CreateTest(ctx, "tester", purityIn)
	if err != nil {
		t.Fatalf("检测登记首次失败: %v", err)
	}
	// 同键重复提交检测登记：应重放首条检测。
	replay, err := e.svc.Purity.CreateTest(ctx, "tester", purityIn)
	if err != nil {
		t.Fatalf("检测登记重放应成功: %v", err)
	}
	if test.ID != replay.ID {
		t.Fatalf("检测登记幂等重放应返回同一检测: %s vs %s", test.ID, replay.ID)
	}

	restockIn := service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 500, IdempotencyKey: key,
	}
	// 回存申请使用相同 key 但属于不同入口：不得命中检测登记的记录，
	// 不得报幂等冲突，也不得重放检测登记的结果。
	rb, err := e.svc.Restock.Create(ctx, "tester", restockIn)
	if err != nil {
		t.Fatalf("回存申请应与检测登记隔离、独立创建成功: %v", err)
	}
	if rb.ID == test.ID {
		t.Fatalf("回存申请不得重放检测登记实体 %s", test.ID)
	}
	// 同键重复提交回存申请：应重放首个回存单。
	rbReplay, err := e.svc.Restock.Create(ctx, "tester", restockIn)
	if err != nil {
		t.Fatalf("回存申请重放应成功: %v", err)
	}
	if rb.ID != rbReplay.ID {
		t.Fatalf("回存申请幂等重放应返回同一回存单: %s vs %s", rb.ID, rbReplay.ID)
	}

	// 两类入口各只创建一条实体，互不污染。
	tests, _ := e.svc.Purity.ListTests(ctx, plan.ID, "", 50)
	if len(tests.Items) != 1 {
		t.Fatalf("检测登记应只有 1 条，实际 %d", len(tests.Items))
	}
	restocks, _ := e.svc.Restock.List(ctx, "", "", 50)
	if len(restocks.Items) != 1 {
		t.Fatalf("回存申请应只有 1 条，实际 %d", len(restocks.Items))
	}

	// 同入口内同键不同请求体仍须报幂等冲突：隔离不应削弱各自语义。
	restockIn.Qty = 999
	_, err = e.svc.Restock.Create(ctx, "tester", restockIn)
	mustErrCode(t, err, "IDEMPOTENCY_CONFLICT")
}
