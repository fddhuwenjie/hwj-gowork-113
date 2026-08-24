package itest

import (
	"testing"
	"time"

	"germplasm/internal/service"
)

// TestOptimisticLock 乐观锁：携带过期版本的更新必须失败，
// 且不会改变任何状态。
func TestOptimisticLock(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(500, 250, "C14")
	e.setupSensors("C14", 2)
	o, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 100,
		RuleVersionID: rule.ID, Deadline: e.clk.Now().Add(24 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	// 使用错误版本审批
	_, err = e.svc.Outbound.Approve(ctx, "tester", o.ID, o.Version+5)
	mustErrCode(t, err, "OPTIMISTIC_LOCK")
	// 状态未变化
	cur, _ := e.svc.Outbound.Get(ctx, o.ID)
	if cur.Status != "PENDING" {
		t.Fatalf("乐观锁失败后状态不应变化，实际 %s", cur.Status)
	}
	// 正确版本成功
	if _, err := e.svc.Outbound.Approve(ctx, "tester", o.ID, cur.Version); err != nil {
		t.Fatalf("正确版本审批应成功: %v", err)
	}
	// 旧版本再次审批（并发重试场景）必须失败
	_, err = e.svc.Outbound.Approve(ctx, "tester", o.ID, cur.Version)
	mustErrCode(t, err, "OPTIMISTIC_LOCK")
}

// TestOptimisticLockOnSample 样本分配库位的乐观锁校验。
func TestOptimisticLockOnSample(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	_, _, batch, loc := e.setupBase(100, 100, "C15")
	_ = batch
	page, err := e.svc.Storage.ListSamples(ctx, "", string("IN_STOCK"), "", 1)
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("查询样本失败: %v", err)
	}
	smp := page.Items[0]
	_, err = e.svc.Storage.AssignLocation(ctx, "tester", smp.ID, loc.ID, smp.Version+9)
	mustErrCode(t, err, "OPTIMISTIC_LOCK")
}
