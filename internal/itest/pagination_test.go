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

// TestSensorPaginationByChamber 按冷库筛选传感器分页：翻页必须保持同一冷库范围，
// 且不重不漏，不能混入其他冷库的设备。游标键应与排序键一致。
func TestSensorPaginationByChamber(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	const chamberA = "COLD-A"
	const chamberB = "COLD-B"
	// 在 A 中注册足够多传感器，并穿插 B 冷库设备，迫使翻页跨页。
	for i := 0; i < 25; i++ {
		if _, err := e.svc.Sensors.CreateSensor(ctx, "tester", service.CreateSensorInput{
			Code: e.unique("TEMP"), Chamber: chamberA, Metric: "TEMPERATURE",
		}); err != nil {
			t.Fatalf("创建 A 冷库传感器失败: %v", err)
		}
		// 穿插其他冷库设备，分页不得混入。
		if _, err := e.svc.Sensors.CreateSensor(ctx, "tester", service.CreateSensorInput{
			Code: e.unique("TEMP"), Chamber: chamberB, Metric: "TEMPERATURE",
		}); err != nil {
			t.Fatalf("创建 B 冷库传感器失败: %v", err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		page, err := e.svc.Sensors.ListSensors(ctx, chamberA, cursor, 10)
		if err != nil {
			t.Fatalf("分页查询失败: %v", err)
		}
		for _, s := range page.Items {
			if s.Chamber != chamberA {
				t.Fatalf("按冷库筛选混入其他冷库设备: %s", s.Chamber)
			}
			if seen[s.ID] {
				t.Fatalf("分页结果重复: %s", s.ID)
			}
			seen[s.ID] = true
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
		t.Fatalf("分页应覆盖 A 冷库全部 25 台，实际 %d", len(seen))
	}
}

// TestSensorPaginationSameTimestamp 同一时间戳下的稳定分页：游标键必须与 ORDER BY 的 id 一致，
// 否则第二页会重复首页设备。
func TestSensorPaginationSameTimestamp(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	// 同一假时钟时刻注册多台传感器，迫使 created_at 相同，依赖 id 二级排序。
	for i := 0; i < 12; i++ {
		if _, err := e.svc.Sensors.CreateSensor(ctx, "tester", service.CreateSensorInput{
			Code: e.unique("TEMP"), Chamber: "COLD-S", Metric: "TEMPERATURE",
		}); err != nil {
			t.Fatalf("创建传感器失败: %v", err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for pages := 0; ; pages++ {
		page, err := e.svc.Sensors.ListSensors(ctx, "COLD-S", cursor, 5)
		if err != nil {
			t.Fatalf("分页查询失败: %v", err)
		}
		for _, s := range page.Items {
			if seen[s.ID] {
				t.Fatalf("同一时间戳分页重复: %s", s.ID)
			}
			seen[s.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
		if pages > 10 {
			t.Fatalf("分页未收敛")
		}
	}
	if len(seen) != 12 {
		t.Fatalf("分页应覆盖全部 12 台，实际 %d", len(seen))
	}
}
