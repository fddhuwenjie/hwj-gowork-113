package itest

import (
	"testing"
	"time"

	"germplasm/internal/domain"
	"germplasm/internal/service"
)

// TestApproveWindowCoverage 校验出库审批的环境时间窗：
// 窗口内温湿度读数必须覆盖全部小时桶且无越限，否则审批失败且事务回滚。
func TestApproveWindowCoverage(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(500, 250, "C04")
	// 不写入任何环境读数：覆盖率不足，审批必须失败。
	o, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 100,
		RuleVersionID: rule.ID, Deadline: e.clk.Now().Add(24 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	_, err = e.svc.Outbound.Approve(ctx, "tester", o.ID, o.Version)
	mustErrCode(t, err, "ENV_WINDOW_VIOLATION")
	// 事务回滚：批次数量不变，申请仍为 PENDING。
	b, _ := e.svc.Storage.GetBatch(ctx, batch.ID)
	if b.QtyFrozen != 0 || b.QtyAvailable != 500 {
		t.Fatalf("审批失败后批次数量不应变化: %+v", b)
	}
	oAfter, _ := e.svc.Outbound.Get(ctx, o.ID)
	if oAfter.Status != domain.OutboundPending {
		t.Fatalf("审批失败后申请应仍为 PENDING，实际 %s", oAfter.Status)
	}
	// 补充达标读数后审批成功。
	e.setupSensors("C04", 2)
	oAfter, _ = e.svc.Outbound.Get(ctx, o.ID)
	if _, err := e.svc.Outbound.Approve(ctx, "tester", o.ID, oAfter.Version); err != nil {
		t.Fatalf("覆盖达标后审批应成功: %v", err)
	}
}

// TestApproveWindowOutOfRange 窗口内存在越限温度读数时审批失败。
func TestApproveWindowOutOfRange(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(500, 250, "C05")
	e.setupSensorsWithValue("C05", 2, -10, 30) // 温度 -10 超出 [-20,-15]
	o, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 100,
		RuleVersionID: rule.ID, Deadline: e.clk.Now().Add(24 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	_, err = e.svc.Outbound.Approve(ctx, "tester", o.ID, o.Version)
	mustErrCode(t, err, "ENV_WINDOW_VIOLATION")
}

// TestFulfillWindowCoverage 校验出库后窗口：审批到出库之间环境必须持续受监控。
func TestFulfillWindowCoverage(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(500, 250, "C06")
	e.setupSensors("C06", 2)
	o, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 100,
		RuleVersionID: rule.ID, Deadline: e.clk.Now().Add(24 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	approved, err := e.svc.Outbound.Approve(ctx, "tester", o.ID, o.Version)
	if err != nil {
		t.Fatalf("审批失败: %v", err)
	}
	// 推进 1 小时但不补充读数：出库后窗口覆盖不足。
	e.clk.Advance(time.Hour)
	_, err = e.svc.Outbound.Fulfill(ctx, "tester", o.ID, approved.Version)
	mustErrCode(t, err, "ENV_WINDOW_VIOLATION")
	// 补充读数后出库成功。
	e.setupSensors("C06", 1)
	current, _ := e.svc.Outbound.Get(ctx, o.ID)
	if _, err := e.svc.Outbound.Fulfill(ctx, "tester", o.ID, current.Version); err != nil {
		t.Fatalf("覆盖达标后出库应成功: %v", err)
	}
}

// TestRuleActivation 同一规则编码仅一个启用版本，旧版本自动退役。
func TestRuleActivation(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	v1, err := e.svc.Rules.CreateRuleVersion(ctx, "tester", ruleInput("STD-ACT"))
	if err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}
	if _, err := e.svc.Rules.ActivateRule(ctx, "tester", v1.ID); err != nil {
		t.Fatalf("启用规则失败: %v", err)
	}
	v2, err := e.svc.Rules.CreateRuleVersion(ctx, "tester", ruleInput("STD-ACT"))
	if err != nil {
		t.Fatalf("创建规则 v2 失败: %v", err)
	}
	if v2.VersionNo != 2 {
		t.Fatalf("规则版本号应自动递增为 2，实际 %d", v2.VersionNo)
	}
	if _, err := e.svc.Rules.ActivateRule(ctx, "tester", v2.ID); err != nil {
		t.Fatalf("启用规则 v2 失败: %v", err)
	}
	v1After, _ := e.svc.Rules.GetRule(ctx, v1.ID)
	if v1After.Status != domain.RuleRetired {
		t.Fatalf("旧版本应退役，实际 %s", v1After.Status)
	}
}
