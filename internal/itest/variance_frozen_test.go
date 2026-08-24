package itest

import (
	"testing"
	"time"

	"germplasm/internal/service"
)

// TestInventoryVarianceFrozenNoFalsePositive 回归测试：出库审批冻结后，
// 批次数量守恒、冻结明细正确，此时库存差异巡检不得把合法冻结报为可用量异常。
// 覆盖审批后（QtyFrozen>0 的活跃冻结态）中间态——旧实现因把冻结量重复扣算
// 而在此态误报，执行或取消出库后 QtyFrozen 归零异常才消失。
func TestInventoryVarianceFrozenNoFalsePositive(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(1000, 250, "CF")
	e.setupSensors("CF", 2)

	// 出库 300，审批冻结：批次守恒、冻结 300、可用 700。
	o, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 300,
		RuleVersionID: rule.ID, BreedingTarget: "目标", Deadline: e.clk.Now().Add(48 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	if _, err := e.svc.Outbound.Approve(ctx, "tester", o.ID, o.Version); err != nil {
		t.Fatalf("审批失败: %v", err)
	}
	b, _ := e.svc.Storage.GetBatch(ctx, batch.ID)
	if b.CheckConservation() != 0 || b.QtyAvailable != 700 || b.QtyFrozen != 300 {
		t.Fatalf("冻结后批次不守恒: %+v", b)
	}
	// 样本侧 300 转为 FROZEN，700 留在 IN_STOCK，互不折叠。
	assertSampleSum(t, e, batch.ID, 700, 300, 0, 0)

	// 库存差异巡检：合法冻结不得制造可用量异常——这是被修复的重复扣算点。
	vars, err := e.svc.Risk.InventoryVariances(ctx)
	if err != nil {
		t.Fatalf("库存差异巡检失败: %v", err)
	}
	for _, v := range vars {
		if v.BatchID == batch.ID {
			t.Fatalf("活跃冻结态不应出现可用量异常: %+v", v)
		}
	}

	// 取消出库：冻结释放，冻结量归零后仍应无异常（确认异常不是“因取消而消失”的回声）。
	oAfter, _ := e.svc.Outbound.Get(ctx, o.ID)
	if _, err := e.svc.Outbound.Cancel(ctx, "tester", o.ID, oAfter.Version); err != nil {
		t.Fatalf("取消出库申请失败: %v", err)
	}
	vars, err = e.svc.Risk.InventoryVariances(ctx)
	if err != nil {
		t.Fatalf("取消后库存差异巡检失败: %v", err)
	}
	for _, v := range vars {
		if v.BatchID == batch.ID {
			t.Fatalf("取消后不应出现可用量异常: %+v", v)
		}
	}
}
