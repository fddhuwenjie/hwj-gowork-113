package itest

import (
	"testing"

	"germplasm/internal/service"
)

// TestStablePagination 稳定分页：连续翻页不重不漏，且中途插入新数据不影响已翻页结果。
func TestStablePagination(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	for i := 0; i < 45; i++ {
		if _, err := e.svc.Resources.CreateResource(ctx, "tester", service.CreateResourceInput{
			Code: e.unique("RES"), Name: "种质", Species: "S", Category: "C",
		}); err != nil {
			t.Fatalf("创建资源失败: %v", err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		page, err := e.svc.Resources.ListResources(ctx, cursor, 20)
		if err != nil {
			t.Fatalf("分页查询失败: %v", err)
		}
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatalf("分页结果重复: %s", item.ID)
			}
			seen[item.ID] = true
		}
		pages++
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
		if pages == 1 {
			// 翻页间隙插入新数据，不应影响剩余翻页的正确性（新数据出现在末尾）。
			if _, err := e.svc.Resources.CreateResource(ctx, "tester", service.CreateResourceInput{
				Code: e.unique("RES"), Name: "新种质", Species: "S", Category: "C",
			}); err != nil {
				t.Fatalf("插入新资源失败: %v", err)
			}
		}
		if pages > 10 {
			t.Fatalf("分页未收敛")
		}
	}
	if len(seen) != 46 {
		t.Fatalf("分页应覆盖全部 46 条，实际 %d", len(seen))
	}
}

// TestBatchPaginationFilter 批次分页与状态过滤。
func TestBatchPaginationFilter(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	res, _ := e.svc.Resources.CreateResource(ctx, "tester", service.CreateResourceInput{
		Code: e.unique("RES"), Name: "种质", Species: "S", Category: "C",
	})
	acc, _ := e.svc.Resources.CreateAccession(ctx, "tester", service.CreateAccessionInput{
		ResourceID: res.ID, AccessionNo: e.unique("ACC"),
	})
	for i := 0; i < 5; i++ {
		if _, err := e.svc.Storage.CreateOriginalBatch(ctx, "tester", service.CreateBatchInput{
			AccessionID: acc.ID, BatchNo: e.unique("BAT"), QtyTotal: 100,
		}); err != nil {
			t.Fatalf("创建批次失败: %v", err)
		}
	}
	page, err := e.svc.Storage.ListBatches(ctx, acc.ID, "ACTIVE", "", 3)
	if err != nil {
		t.Fatalf("查询批次失败: %v", err)
	}
	if len(page.Items) != 3 || page.NextCursor == "" {
		t.Fatalf("首页应为 3 条且有下页游标")
	}
	page2, err := e.svc.Storage.ListBatches(ctx, acc.ID, "ACTIVE", page.NextCursor, 3)
	if err != nil {
		t.Fatalf("查询第二页失败: %v", err)
	}
	if len(page2.Items) != 2 || page2.NextCursor != "" {
		t.Fatalf("第二页应为 2 条且无下页游标")
	}
}

// TestAccessionFilterPagination 跨页筛选隔离：按 resource_id 筛选 accession 并翻页时，
// 每一页都必须只含目标资源下的登记记录，游标不得清除 resource_id 隔离，
// 且翻页结果不重不漏。
func TestAccessionFilterPagination(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx

	// 资源 A：25 条 accession（跨多页，limit 取 10 → 3 页）
	resA, _ := e.svc.Resources.CreateResource(ctx, "tester", service.CreateResourceInput{
		Code: e.unique("RES"), Name: "资源A", Species: "S", Category: "C",
	})
	// 资源 B：30 条 accession，穿插在 A 之后创建，时间排序上与 A 混排。
	resB, _ := e.svc.Resources.CreateResource(ctx, "tester", service.CreateResourceInput{
		Code: e.unique("RES"), Name: "资源B", Species: "S", Category: "C",
	})
	for i := 0; i < 25; i++ {
		if _, err := e.svc.Resources.CreateAccession(ctx, "tester", service.CreateAccessionInput{
			ResourceID: resA.ID, AccessionNo: e.unique("ACC"),
		}); err != nil {
			t.Fatalf("创建资源A的accession失败: %v", err)
		}
		// 资源B的accession交错插入，确保时间排序上与A交错，放大跨页漏筛风险。
		if _, err := e.svc.Resources.CreateAccession(ctx, "tester", service.CreateAccessionInput{
			ResourceID: resB.ID, AccessionNo: e.unique("ACC"),
		}); err != nil {
			t.Fatalf("创建资源B的accession失败: %v", err)
		}
	}

	const limit = 10
	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		page, err := e.svc.Resources.ListAccessions(ctx, resA.ID, cursor, limit)
		if err != nil {
			t.Fatalf("分页查询失败: %v", err)
		}
		for _, item := range page.Items {
			if item.ResourceID != resA.ID {
				t.Fatalf("跨页筛选泄漏：资源A筛选页混入资源 %s 的登记记录", item.ResourceID)
			}
			if seen[item.ID] {
				t.Fatalf("分页结果重复: %s", item.ID)
			}
			seen[item.ID] = true
		}
		pages++
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
		if pages > 10 {
			t.Fatalf("分页未收敛")
		}
	}
	if len(seen) != 25 {
		t.Fatalf("资源A应覆盖全部 25 条 accession，实际 %d", len(seen))
	}
}
