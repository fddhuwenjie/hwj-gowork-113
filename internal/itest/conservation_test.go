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

// injectPhantomUnit 人为在一号在库样本上加 1，制造“账面 10 / 样本汇总 11”
// 的盘亏（样本记录多记 1，物理已损耗）。复核盘亏销毁据此纠正。
// 返回注入前的快照（qty, version）以便审批失败后核对样本未变。
func injectPhantomUnit(t *testing.T, e *testEnv, batchID string) domain.Sample {
	t.Helper()
	page, err := e.svc.Storage.ListSamples(e.ctx, batchID, string(domain.SampleInStock), "", 1)
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("查询在库样本失败: %v", err)
	}
	s := page.Items[0]
	if _, err := e.db.ExecContext(e.ctx,
		`UPDATE samples SET qty=qty+1, version=version+1 WHERE id=?`, s.ID); err != nil {
		t.Fatalf("注入盘亏样本失败: %v", err)
	}
	after, err := e.svc.Storage.GetSample(e.ctx, s.ID)
	if err != nil {
		t.Fatalf("查询注入后样本失败: %v", err)
	}
	return *after
}

// snapshotBatch 记录审批前的批次账面，用于审批失败后的“零变更”断言。
func snapshotBatch(t *testing.T, e *testEnv, batchID string) domain.Batch {
	t.Helper()
	b, err := e.svc.Storage.GetBatch(e.ctx, batchID)
	if err != nil {
		t.Fatalf("查询批次失败: %v", err)
	}
	return *b
}

// assertNoInventoryVariance 断言该批次在库存差异巡检中无异常。
func assertNoInventoryVariance(t *testing.T, e *testEnv, batchID string) {
	t.Helper()
	vars, err := e.svc.Risk.InventoryVariances(e.ctx)
	if err != nil {
		t.Fatalf("库存差异巡检失败: %v", err)
	}
	for _, v := range vars {
		if v.BatchID == batchID {
			t.Fatalf("批次不应出现库存差异: %+v", v)
		}
	}
}

// inventoryVarianceOf 返回指定批次的库存差异快照，未发现时返回零值指针。
func inventoryVarianceOf(t *testing.T, e *testEnv, batchID string) *service.InventoryVariance {
	t.Helper()
	vars, err := e.svc.Risk.InventoryVariances(e.ctx)
	if err != nil {
		t.Fatalf("库存差异巡检失败: %v", err)
	}
	for i := range vars {
		if vars[i].BatchID == batchID {
			return &vars[i]
		}
	}
	return nil
}

// TestDestructionReconcileFailureAtomicity 复核盘亏销毁审批失败必须整体回滚，
// 不得遗留任何样本变更。回归：盘亏扣减原先在事务外自动提交，审批因数量不足
// 回滚后样本数量/版本已变而批次账面未同步，账实脱节。
func TestDestructionReconcileFailureAtomicity(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(10, 5, "CR")
	e.setupSensors("CR", 2)

	// 注入 1 个盘亏：账面 10，样本汇总 11。复核盘亏销毁应销毁 10（含 1 盘亏）。
	phantom := injectPhantomUnit(t, e, batch.ID)

	// 提交复核盘亏销毁：qty=9（存储 9，读取时 +1=10）。创建校验账面 10 ≥ 9 通过。
	d, err := e.svc.Destruction.Create(ctx, "tester", service.CreateDestructionInput{
		BatchID: batch.ID, Qty: 9, Reason: "复核盘亏销毁",
	})
	if err != nil {
		t.Fatalf("创建销毁申请失败: %v", err)
	}
	if got := d.Qty; got != 9 {
		t.Fatalf("销毁数量存储值应为 9，实际 %d", got)
	}

	// 另一笔出库先占用 5（冻结），账面可用量降至 5，不足以销毁 10。
	before := snapshotBatch(t, e, batch.ID)
	o, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 5,
		RuleVersionID: rule.ID, BreedingTarget: "目标", Deadline: e.clk.Now().Add(48 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	if _, err := e.svc.Outbound.Approve(ctx, "tester", o.ID, o.Version); err != nil {
		t.Fatalf("出库审批失败: %v", err)
	}

	// 记录销毁审批前的样本状态，用于失败后“零变更”断言。
	sampleBefore, _ := e.svc.Storage.GetSample(ctx, phantom.ID)
	// 记录销毁审批前的库存差异快照（出库冻结核法拆分后存在 -1 盘亏差异）。
	varBefore := inventoryVarianceOf(t, e, batch.ID)

	// 批准复核盘亏销毁：账面可用量 5 < 销毁数量 10，必须失败。
	_, err = e.svc.Destruction.Approve(ctx, "tester", d.ID, d.Version)
	mustErrCode(t, err, "QUANTITY_VIOLATION")

	// 失败后：批次账面与出库审批后一致，未被销毁审批触碰。
	after, _ := e.svc.Storage.GetBatch(ctx, batch.ID)
	if after.QtyAvailable != before.QtyAvailable-5 || after.QtyFrozen != before.QtyFrozen+5 ||
		after.QtyDestroyed != 0 || after.Version != before.Version+1 ||
		after.CheckConservation() != 0 {
		t.Fatalf("失败审批不应改变批次账面: before=%+v after=%+v", before, after)
	}
	// 失败后：盘亏样本数量与版本完全不变（事务回滚覆盖了样本扣减）。
	smp, _ := e.svc.Storage.GetSample(ctx, sampleBefore.ID)
	if smp.Qty != sampleBefore.Qty || smp.Version != sampleBefore.Version {
		t.Fatalf("失败审批不应改变样本: 期望(qty=%d v=%d) 实际(qty=%d v=%d)",
			sampleBefore.Qty, sampleBefore.Version, smp.Qty, smp.Version)
	}
	// 销毁审批单仍为 PENDING，可凭原版本重试。
	cur, _ := e.svc.Destruction.Get(ctx, d.ID)
	if cur.Status != domain.DestructionPending || cur.Version != d.Version {
		t.Fatalf("失败审批后审批单应为 PENDING 且版本不变: %+v", cur)
	}
	// 账实差异与审批前完全一致：失败审批不得改变账面或样本汇总。
	varAfter := inventoryVarianceOf(t, e, batch.ID)
	if (varBefore == nil) != (varAfter == nil) {
		t.Fatalf("失败审批不应改变库存差异是否存在: before=%v after=%v", varBefore, varAfter)
	}
	if varBefore != nil {
		if varAfter.AvailableDiff != varBefore.AvailableDiff ||
			varAfter.SampleSum != varBefore.SampleSum ||
			varAfter.BookSum != varBefore.BookSum ||
			varAfter.Conserved != varBefore.Conserved {
			t.Fatalf("失败审批不应改变库存差异: before=%+v after=%+v", varBefore, varAfter)
		}
	}
}

// TestDestructionReconcileSuccessConserved 复核盘亏销毁审批成功时账实守恒：
// 盘亏扣减与 FIFO 销毁、批次账面扣减在同一事务内完成。
func TestDestructionReconcileSuccessConserved(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	_, _, batch, _ := e.setupBase(10, 5, "CS")

	// 注入 1 个盘亏：账面 10，样本汇总 11。
	phantom := injectPhantomUnit(t, e, batch.ID)

	d, err := e.svc.Destruction.Create(ctx, "tester", service.CreateDestructionInput{
		BatchID: batch.ID, Qty: 9, Reason: "复核盘亏销毁",
	})
	if err != nil {
		t.Fatalf("创建销毁申请失败: %v", err)
	}
	if _, err := e.svc.Destruction.Approve(ctx, "tester", d.ID, d.Version); err != nil {
		t.Fatalf("批准复核盘亏销毁失败: %v", err)
	}

	// 全部可用库存销毁，批次转为 DESTROYED 且守恒。
	b, _ := e.svc.Storage.GetBatch(ctx, batch.ID)
	if b.Status != domain.BatchDestroyed ||
		b.QtyAvailable != 0 || b.QtyDestroyed != 10 || b.CheckConservation() != 0 {
		t.Fatalf("成功审批后批次不守恒: %+v", b)
	}
	// 盘亏样本已随之销毁或扣减，账实一致。
	assertSampleSum(t, e, batch.ID, 0, 0, 0, 10)
	assertNoInventoryVariance(t, e, batch.ID)
	_ = phantom
}
