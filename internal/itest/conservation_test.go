package itest

import (
	"testing"
	"time"

	"germplasm/internal/domain"
	"germplasm/internal/service"
)

// TestQuantityConservation 校验冻结、出库、取消与销毁全过程中的数量守恒。
func TestQuantityConservation(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(1000, 250, "C02")
	e.setupSensors("C02", 2)

	// 出库 300：审批冻结后批次守恒。
	o, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 300,
		RuleVersionID: rule.ID, BreedingTarget: "目标", Deadline: e.clk.Now().Add(48 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	approved, err := e.svc.Outbound.Approve(ctx, "tester", o.ID, o.Version)
	if err != nil {
		t.Fatalf("审批失败: %v", err)
	}
	b, _ := e.svc.Storage.GetBatch(ctx, batch.ID)
	if b.CheckConservation() != 0 || b.QtyAvailable != 700 || b.QtyFrozen != 300 {
		t.Fatalf("冻结后批次不守恒: %+v", b)
	}
	// 样本汇总必须与账面一致。
	assertSampleSum(t, e, batch.ID, 700, 300, 0, 0)

	// 取消后冻结释放，数量回补。
	if _, err := e.svc.Outbound.Cancel(ctx, "tester", o.ID, approved.Version); err != nil {
		t.Fatalf("取消出库申请失败: %v", err)
	}
	b, _ = e.svc.Storage.GetBatch(ctx, batch.ID)
	if b.CheckConservation() != 0 || b.QtyAvailable != 1000 || b.QtyFrozen != 0 {
		t.Fatalf("取消后批次不守恒: %+v", b)
	}
	assertSampleSum(t, e, batch.ID, 1000, 0, 0, 0)

	// 再次出库并完成，随后销毁部分库存。
	o2, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 400,
		RuleVersionID: rule.ID, BreedingTarget: "目标", Deadline: e.clk.Now().Add(48 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	if _, err := e.svc.Outbound.Approve(ctx, "tester", o2.ID, o2.Version); err != nil {
		t.Fatalf("审批失败: %v", err)
	}
	o2f, _ := e.svc.Outbound.Get(ctx, o2.ID)
	e.clk.Advance(30 * time.Minute)
	e.setupSensors("C02", 1)
	if _, err := e.svc.Outbound.Fulfill(ctx, "tester", o2.ID, o2f.Version); err != nil {
		t.Fatalf("出库失败: %v", err)
	}
	b, _ = e.svc.Storage.GetBatch(ctx, batch.ID)
	if b.CheckConservation() != 0 || b.QtyAvailable != 600 || b.QtyOutbound != 400 {
		t.Fatalf("出库后批次不守恒: %+v", b)
	}
	assertSampleSum(t, e, batch.ID, 600, 0, 400, 0)

	d, err := e.svc.Destruction.Create(ctx, "tester", service.CreateDestructionInput{
		BatchID: batch.ID, Qty: 200, Reason: "活力丧失",
	})
	if err != nil {
		t.Fatalf("创建销毁申请失败: %v", err)
	}
	if _, err := e.svc.Destruction.Approve(ctx, "tester", d.ID, d.Version); err != nil {
		t.Fatalf("批准销毁失败: %v", err)
	}
	b, _ = e.svc.Storage.GetBatch(ctx, batch.ID)
	if b.CheckConservation() != 0 || b.QtyAvailable != 400 || b.QtyDestroyed != 200 {
		t.Fatalf("销毁后批次不守恒: %+v", b)
	}
	assertSampleSum(t, e, batch.ID, 400, 0, 400, 200)

	// 库存差异巡检应无异常。
	vars, err := e.svc.Risk.InventoryVariances(ctx)
	if err != nil {
		t.Fatalf("库存差异巡检失败: %v", err)
	}
	for _, v := range vars {
		if v.BatchID == batch.ID {
			t.Fatalf("批次不应出现库存差异: %+v", v)
		}
	}
}

// assertSampleSum 校验批次样本按状态汇总与账面一致。
func assertSampleSum(t *testing.T, e *testEnv, batchID string, inStock, frozen, outbound, destroyed int64) {
	t.Helper()
	sums, err := e.svc.Repos.Samples.SumByBatchAndStatus(e.ctx, e.db, batchID)
	if err != nil {
		t.Fatalf("汇总样本失败: %v", err)
	}
	if sums[domain.SampleInStock] != inStock || sums[domain.SampleFrozen] != frozen ||
		sums[domain.SampleOutbound] != outbound || sums[domain.SampleDestroyed] != destroyed {
		t.Fatalf("样本汇总与账面不一致: 期望(%d,%d,%d,%d) 实际%v",
			inStock, frozen, outbound, destroyed, sums)
	}
}

// TestOutboundInsufficientQty 出库数量超过可用量时创建即拒绝，且不产生任何写入。
func TestOutboundInsufficientQty(t *testing.T) {
	e := newTestEnv(t)
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(100, 50, "C03")
	_, err := e.svc.Outbound.Create(e.ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 200,
		RuleVersionID: rule.ID, Deadline: e.clk.Now().Add(time.Hour).Format(time.RFC3339Nano),
	})
	mustErrCode(t, err, "QUANTITY_VIOLATION")
	page, err := e.svc.Outbound.List(e.ctx, "", "", 50)
	if err != nil {
		t.Fatalf("查询出库申请失败: %v", err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("失败申请不应写入，实际 %d 条", len(page.Items))
	}
}
