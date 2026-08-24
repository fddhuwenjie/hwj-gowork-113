package itest

import (
	"testing"
	"time"

	"germplasm/internal/domain"
	"germplasm/internal/service"
)

// createTestsInOrder 在同一繁育计划下连续登记 n 次检测，每次推进时钟保证
// created_at 严格递增（且 tested_at 缺省取 created_at），返回按登记顺序的检测。
func (e *testEnv) createTestsInOrder(t *testing.T, planID string, n int) []*domain.PurityTest {
	t.Helper()
	var out []*domain.PurityTest
	for i := 0; i < n; i++ {
		e.clk.Advance(time.Minute)
		tt, err := e.svc.Purity.CreateTest(e.ctx, "tester", service.CreateTestInput{
			PlanID: planID, SampleQty: 100, CoverageRatio: 1.0, PurityRate: 0.98,
		})
		if err != nil {
			t.Fatalf("登记检测失败: %v", err)
		}
		out = append(out, tt)
	}
	return out
}

// TestPurityListStableAscendingOrder 同一繁育计划连续登记多次检测，
// 检测历史必须按创建时间稳定升序返回：先登记的排前面。
func TestPurityListStableAscendingOrder(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C20")
	created := e.createTestsInOrder(t, plan.ID, 4)

	page, err := e.svc.Purity.ListTests(ctx, plan.ID, "", 10)
	if err != nil {
		t.Fatalf("ListTests 失败: %v", err)
	}
	if len(page.Items) != len(created) {
		t.Fatalf("应返回 %d 条检测，实际 %d", len(created), len(page.Items))
	}
	for i, item := range page.Items {
		if item.ID != created[i].ID {
			t.Fatalf("第 %d 条应为先登记的 %s，实际 %s（顺序非创建升序）",
				i, created[i].ID, item.ID)
		}
	}
}

// TestPurityListCursorNoSkipNoDup 翻页游标应不重不漏、按创建升序遍历全部检测。
func TestPurityListCursorNoSkipNoDup(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C21")
	created := e.createTestsInOrder(t, plan.ID, 5)

	seen := make(map[string]int, len(created))
	var order []string
	cursor := ""
	for {
		page, err := e.svc.Purity.ListTests(ctx, plan.ID, cursor, 2)
		if err != nil {
			t.Fatalf("ListTests 失败: %v", err)
		}
		// 单页不得返回超过请求 limit 的记录。
		if len(page.Items) > 2 {
			t.Fatalf("单页应至多 2 条，实际 %d 条", len(page.Items))
		}
		for _, item := range page.Items {
			seen[item.ID]++
			order = append(order, item.ID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}

	if len(order) != len(created) {
		t.Fatalf("应遍历 %d 条，实际 %d 条（漏掉或重复）", len(created), len(order))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("检测 %s 出现 %d 次，应恰好 1 次", id, n)
		}
	}
	for i, id := range order {
		if id != created[i].ID {
			t.Fatalf("第 %d 条顺序错误：期望 %s，实际 %s", i, created[i].ID, id)
		}
	}
}

// TestPurityListOrdersByCreatedAtNotTestedAt 连续登记两次检测，后登记的那条
// tested_at 更晚。若按 tested_at DESC 排序，后登记的会排到前面；按 created_at
// 升序则先登记的排前面。这里同时构造一条迟到检测（created_at 最晚但
// tested_at 最早），彻底分离两种排序键：只有 created_at 升序能正确还原登记顺序。
func TestPurityListOrdersByCreatedAtNotTestedAt(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C22")

	base := e.clk.Now()
	// 第一条：登记最早，tested_at 取最早时刻。
	e.clk.Set(base)
	first, err := e.svc.Purity.CreateTest(ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 100, CoverageRatio: 1.0, PurityRate: 0.98,
		TestedAt: base.Add(-2 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("登记首条检测失败: %v", err)
	}
	// 第二条：登记居中，tested_at 最晚（按 tested_at DESC 会排到第一）。
	e.clk.Set(base.Add(time.Minute))
	second, err := e.svc.Purity.CreateTest(ctx, "tester", service.CreateTestInput{
		PlanID: plan.ID, SampleQty: 100, CoverageRatio: 1.0, PurityRate: 0.98,
		TestedAt: base.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("登记第二条检测失败: %v", err)
	}
	if !second.TestedAt.After(first.TestedAt) {
		t.Fatalf("前置条件不满足：第二条 tested_at 应晚于首条")
	}

	page, err := e.svc.Purity.ListTests(ctx, plan.ID, "", 10)
	if err != nil {
		t.Fatalf("ListTests 失败: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("应返回 2 条检测，实际 %d", len(page.Items))
	}
	// 期望 created_at 升序：先登记的 first 排首条。
	if page.Items[0].ID != first.ID {
		t.Fatalf("首条应为先登记的 %s，实际 %s（疑似按 tested_at 排序）",
			first.ID, page.Items[0].ID)
	}
	if page.Items[1].ID != second.ID {
		t.Fatalf("第二条应为后登记的 %s，实际 %s", second.ID, page.Items[1].ID)
	}
}
