package itest

import (
	"testing"

	"germplasm/internal/domain"
	"germplasm/internal/service"
)

// TestLocationRisksAggregatesAllActiveRules 风险聚合必须对照全部活动规则，
// 而非仅首个规则。构造两条不同 code 的活动规则：
//   - 规则 A（STD-SEED）：温度 [-20,-15]
//   - 规则 B（STD-STRICT）：温度 [-20,-16]，比 A 更严
//
// 写入一条 -15.5℃ 读数：它在 A 范围内、却违反 B。修复前 ListActive
// 仅返回一条规则、聚合又截断为首个，于是该读数被漏报为无风险；
// 修复后巡检必须报出该库位的越限读数。
func TestLocationRisksAggregatesAllActiveRules(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx

	// 规则 A：默认 STD-SEED，温度 [-20,-15]。
	e.setupRule()

	// 规则 B：温度 [-20,-16]，更严；与 A 不同 code，因此可同时启用。
	v2, err := e.svc.Rules.CreateRuleVersion(ctx, "tester", service.CreateRuleInput{
		Code: "STD-STRICT", MinTemp: -20, MaxTemp: -16,
		MinHumidity: 20, MaxHumidity: 40,
		WindowBeforeHours: 2, WindowAfterHours: 2, MinCoverage: 0.9, MinPurity: 0.95,
	})
	if err != nil {
		t.Fatalf("创建规则 B 失败: %v", err)
	}
	if _, err := e.svc.Rules.ActivateRule(ctx, "tester", v2.ID); err != nil {
		t.Fatalf("启用规则 B 失败: %v", err)
	}

	// 仓储层：两条不同 code 的活动规则都必须返回。
	active, err := e.svc.Repos.Rules.ListActive(ctx, e.db)
	if err != nil {
		t.Fatalf("ListActive 失败: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("ListActive 应返回 2 条活动规则（每 code 一条），实际 %d", len(active))
	}

	// -15.5℃ 在 A [-20,-15] 内，却违反 B [-20,-16]。
	const chamber = "C-RISK"
	e.setupSensorsWithValue(chamber, 0, -15.5, 30)

	risks, err := e.svc.Risk.LocationRisks(ctx)
	if err != nil {
		t.Fatalf("LocationRisks 失败: %v", err)
	}
	var risk *service.LocationRisk
	for i := range risks {
		if risks[i].Chamber == chamber {
			risk = &risks[i]
			break
		}
	}
	if risk == nil {
		t.Fatalf("库位 %s 应被报为有风险（读数违反规则 B），但未出现在巡检结果中", chamber)
	}
	if risk.OutOfRangeCount == 0 {
		t.Fatalf("库位 %s 越限读数应为 1（仅违反规则 B 的读数被漏报即为本 bug），实际 %d",
			chamber, risk.OutOfRangeCount)
	}
}

// TestLocationRisksSingleRuleNotViolated 单规则且读数达标时不报风险，
// 防止修复后聚合逻辑误报。
func TestLocationRisksSingleRuleNotViolated(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	e.setupRule()
	const chamber = "C-OK"
	e.setupSensorsWithValue(chamber, 0, -18, 30) // -18 在 [-20,-15] 内

	risks, err := e.svc.Risk.LocationRisks(ctx)
	if err != nil {
		t.Fatalf("LocationRisks 失败: %v", err)
	}
	for _, r := range risks {
		if r.Chamber == chamber {
			t.Fatalf("库位 %s 读数达标，不应被报为风险，实际越限 %d", chamber, r.OutOfRangeCount)
		}
	}
}

// TestEnvAlertScanMultipleRules 环境告警作业对照全部活动规则，
// 仅违反后续规则的读数同样产生告警。
func TestEnvAlertScanMultipleRules(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	e.setupRule() // A: [-20,-15]
	v2, err := e.svc.Rules.CreateRuleVersion(ctx, "tester", service.CreateRuleInput{
		Code: "STD-STRICT", MinTemp: -20, MaxTemp: -16,
		MinHumidity: 20, MaxHumidity: 40,
		WindowBeforeHours: 2, WindowAfterHours: 2, MinCoverage: 0.9, MinPurity: 0.95,
	})
	if err != nil {
		t.Fatalf("创建规则 B 失败: %v", err)
	}
	if _, err := e.svc.Rules.ActivateRule(ctx, "tester", v2.ID); err != nil {
		t.Fatalf("启用规则 B 失败: %v", err)
	}
	const chamber = "C-ALERT"
	e.setupSensorsWithValue(chamber, 0, -15.5, 30) // 仅违反 B

	e.sched.RunOnceForTest(ctx)

	page, err := e.svc.Repos.Alerts.List(ctx, e.db, "OPEN", domain.AlertEnvOutOfRange, "", 50)
	if err != nil {
		t.Fatalf("查询告警失败: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatalf("读数违反规则 B 应产生环境越限告警，但无告警")
	}
}
