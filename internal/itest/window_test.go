package itest

import (
	"testing"
	"time"

	"germplasm/internal/apperr"
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

// TestApproveWindowStartReadingInclusive 校验窗口契约的闭区间精度边界：
// 出库前 1 小时窗口下，温湿度读数恰好落在窗口起点时必须被保留，审批应成功。
// 回归此前仓库与服务曾对起点施加 +1ns 偏移：当窗口起点带亚秒分量时，+1ns 会把
// 恰好落在起点的读数排除到窗口之外，使审批报告覆盖率为零。
func TestApproveWindowStartReadingInclusive(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	// 将时钟设为带亚秒分量的时刻，使窗口起点 now-1h 同样带亚秒分量；
	// 这样 +1ns 的边界偏移才会真正把起点读数排除（整秒起点会因 RFC3339Nano
	// 省略小数部分而 lex 翻盘，掩盖问题）。
	e.clk.Set(time.Date(2026, 1, 1, 8, 0, 0, 7, time.UTC))
	// 出库前 1 小时窗口：审批前校验固定以 beforeHours 窗口、afterHours=0 评估。
	rv, err := e.svc.Rules.CreateRuleVersion(ctx, "tester", service.CreateRuleInput{
		Code: "STD-EDGE", MinTemp: -20, MaxTemp: -15, MinHumidity: 20, MaxHumidity: 40,
		WindowBeforeHours: 1, WindowAfterHours: 2, MinCoverage: 1.0, MinPurity: 0.95,
	})
	if err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}
	rule, err := e.svc.Rules.ActivateRule(ctx, "tester", rv.ID)
	if err != nil {
		t.Fatalf("启用规则失败: %v", err)
	}
	_, acc, batch, _ := e.setupBase(500, 250, "C21")
	tempSensor, err := e.svc.Sensors.CreateSensor(ctx, "tester", service.CreateSensorInput{
		Code: e.unique("TEMP"), Chamber: "C21", Metric: "TEMPERATURE",
	})
	if err != nil {
		t.Fatalf("创建温度传感器失败: %v", err)
	}
	humSensor, err := e.svc.Sensors.CreateSensor(ctx, "tester", service.CreateSensorInput{
		Code: e.unique("HUM"), Chamber: "C21", Metric: "HUMIDITY",
	})
	if err != nil {
		t.Fatalf("创建湿度传感器失败: %v", err)
	}
	// 窗口仅一个桶 [start, now)，其唯一读数恰好落在起点 now-1h。
	// 若起点被排除，该桶即无覆盖、覆盖率归零，复现报告的边界缺陷。
	now := e.clk.Now()
	start := now.Add(-1 * time.Hour)
	for _, p := range []struct {
		id string
		v  float64
	}{{tempSensor.ID, -18}, {humSensor.ID, 30}} {
		if _, err := e.svc.Sensors.AddReading(ctx, p.id, service.AddReadingInput{
			Value: p.v, RecordedAt: start.Format(time.RFC3339Nano),
		}); err != nil {
			t.Fatalf("写入环境读数失败: %v", err)
		}
	}
	o, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 100,
		RuleVersionID: rule.ID, Deadline: e.clk.Now().Add(24 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	if _, err := e.svc.Outbound.Approve(ctx, "tester", o.ID, o.Version); err != nil {
		if ae, ok := err.(*apperr.Error); ok && ae.Code == "ENV_WINDOW_VIOLATION" {
			t.Fatalf("读数落在窗口起点应被闭区间保留，审批却判定覆盖不足：%v", ae)
		}
		t.Fatalf("审批失败: %v", err)
	}
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
