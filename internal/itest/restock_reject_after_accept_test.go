package itest

import (
	"testing"
	"time"

	"germplasm/internal/domain"
	"germplasm/internal/service"
)

// acceptRestock 完成出库、繁育、合格检测与回存验收通过，返回验收通过后的验收单。
func (e *testEnv) acceptRestock(t *testing.T, chamber string) *domain.RestockBatch {
	t.Helper()
	ctx := e.ctx
	plan := e.breedToTest(t, chamber)
	test, err := e.svc.Purity.CreateTest(ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 300, CoverageRatio: 1.0, PurityRate: 0.98,
		TestedAt: e.clk.Now().Format(time.RFC3339Nano),
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
		t.Fatalf("回存验收失败: %v", err)
	}
	return accepted
}

// TestRestockRejectAfterAccept 验收通过为不可逆终态：复核人员再次驳回必须失败，
// 新批次、谱系、母批状态、繁育计划与验收单的 new_batch_id 引用均须保持不变。
func TestRestockRejectAfterAccept(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	accepted := e.acceptRestock(t, "C30")
	newBatchID := accepted.NewBatchID
	if newBatchID == "" {
		t.Fatalf("验收通过后应生成新批次")
	}

	// 复核人员再次执行驳回：验收成功本应是不可逆终态，必须被状态机拒绝。
	_, err := e.svc.Restock.Reject(ctx, "reviewer", accepted.ID, "反悔", accepted.Version)
	mustErrCode(t, err, "STATE_CONFLICT")

	// 验收单仍为 ACCEPTED，new_batch_id 引用不得被清空。
	still, err := e.svc.Restock.Get(ctx, accepted.ID)
	if err != nil {
		t.Fatalf("查询验收单失败: %v", err)
	}
	if still.Status != domain.RestockAccepted {
		t.Fatalf("驳回被拒后状态应仍为 ACCEPTED，实际 %s", still.Status)
	}
	if still.NewBatchID != newBatchID {
		t.Fatalf("new_batch_id 引用不得消失: 期望 %s，实际 %s", newBatchID, still.NewBatchID)
	}

	// 新批次继续存在且属性不变（引用不得被"悬空"）。
	nb, err := e.svc.Storage.GetBatch(ctx, newBatchID)
	if err != nil {
		t.Fatalf("新批次应继续存在: %v", err)
	}
	if nb.Kind != domain.BatchRestock || nb.QtyTotal != 800 {
		t.Fatalf("新批次属性异常: %+v", nb)
	}

	// 谱系边仍在。
	lineage, err := e.svc.Lineage.GetLineage(ctx, newBatchID)
	if err != nil || len(lineage.Parents) != 1 {
		t.Fatalf("谱系关联应保持完整: %+v err=%v", lineage, err)
	}
}

// TestRestockRejectPending PENDING 状态下驳回仍正常工作。
func TestRestockRejectPending(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C31")
	rb, err := e.svc.Restock.Create(ctx, "tester", service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 800,
	})
	if err != nil {
		t.Fatalf("创建回存单失败: %v", err)
	}
	rejected, err := e.svc.Restock.Reject(ctx, "tester", rb.ID, "纯度存疑", rb.Version)
	if err != nil {
		t.Fatalf("PENDING 驳回应成功: %v", err)
	}
	if rejected.Status != domain.RestockRejected || rejected.RejectReason != "纯度存疑" {
		t.Fatalf("驳回结果异常: %+v", rejected)
	}
}

// TestRestockRejectTwice REJECTED 为终态，重复驳回应被状态机拒绝。
func TestRestockRejectTwice(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C32")
	rb, err := e.svc.Restock.Create(ctx, "tester", service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 800,
	})
	if err != nil {
		t.Fatalf("创建回存单失败: %v", err)
	}
	if _, err := e.svc.Restock.Reject(ctx, "tester", rb.ID, "第一次", rb.Version); err != nil {
		t.Fatalf("首次驳回失败: %v", err)
	}
	cur, _ := e.svc.Restock.Get(ctx, rb.ID)
	_, err = e.svc.Restock.Reject(ctx, "tester", rb.ID, "第二次", cur.Version)
	mustErrCode(t, err, "STATE_CONFLICT")
}
