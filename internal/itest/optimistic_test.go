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

// TestArchiveResourceUsesInjectedClock ensures resource state and audit work
// from the same deterministic business timestamp.
func TestArchiveResourceUsesInjectedClock(t *testing.T) {
	e := newTestEnv(t)
	created, err := e.svc.Resources.CreateResource(e.ctx, "tester", service.CreateResourceInput{
		Code: "RES-CLOCK", Name: "时钟校验", Species: "Oryza sativa",
	})
	if err != nil {
		t.Fatalf("创建资源失败: %v", err)
	}

	e.clk.Advance(6 * time.Hour)
	want := e.clk.Now()
	archived, err := e.svc.Resources.ArchiveResource(e.ctx, "tester", created.ID, created.Version)
	if err != nil {
		t.Fatalf("归档资源失败: %v", err)
	}
	if !archived.UpdatedAt.Equal(want) {
		t.Fatalf("归档时间必须来自注入时钟: got %s want %s", archived.UpdatedAt, want)
	}

	stored, err := e.svc.Resources.GetResource(e.ctx, created.ID)
	if err != nil {
		t.Fatalf("读取归档资源失败: %v", err)
	}
	if !stored.UpdatedAt.Equal(want) {
		t.Fatalf("持久化归档时间必须来自注入时钟: got %s want %s", stored.UpdatedAt, want)
	}
}

// TestArchiveAuditBelongsToResource 归档与创建必须共享同一实体身份
// （entity_type=resource、entity_id=资源 ID），否则按 resource 类型与资源 ID
// 检索审计时只能看到创建记录，归档事件会错挂到 accession 类型 / 资源编码下。
func TestArchiveAuditBelongsToResource(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	created, err := e.svc.Resources.CreateResource(ctx, "tester", service.CreateResourceInput{
		Code: "RES-AUDIT", Name: "审计归属校验", Species: "Oryza sativa",
	})
	if err != nil {
		t.Fatalf("创建资源失败: %v", err)
	}

	if _, err := e.svc.Resources.ArchiveResource(ctx, "tester", created.ID, created.Version); err != nil {
		t.Fatalf("归档资源失败: %v", err)
	}

	// 按 resource 类型 + 资源 ID 检索，必须同时命中 create 与 archive 两条事件。
	entries, _, err := e.svc.Audit.List(ctx, e.svc.Tx.DB(), "resource", created.ID, "", 100)
	if err != nil {
		t.Fatalf("查询审计失败: %v", err)
	}
	var gotCreate, gotArchive bool
	for _, en := range entries {
		if en.EntityType != "resource" || en.EntityID != created.ID {
			t.Fatalf("归档审计归属错乱: 期望 resource/%s，实际 %s/%s", created.ID, en.EntityType, en.EntityID)
		}
		switch en.Action {
		case "resource.create":
			gotCreate = true
		case "resource.archive":
			gotArchive = true
		}
	}
	if !gotCreate || !gotArchive {
		t.Fatalf("resource/%s 审计应含 create 与 archive，实际 create=%v archive=%v", created.ID, gotCreate, gotArchive)
	}

	// 归档事件不得挂在 accession 类型或资源编码（Code）下。
	if entries, _, err := e.svc.Audit.List(ctx, e.svc.Tx.DB(), "accession", created.ID, "", 100); err != nil {
		t.Fatalf("查询 accession 审计失败: %v", err)
	} else {
		for _, en := range entries {
			if en.Action == "resource.archive" {
				t.Fatalf("归档事件错挂到 accession 类型下: %s/%s", en.EntityType, en.EntityID)
			}
		}
	}
	if entries, _, err := e.svc.Audit.List(ctx, e.svc.Tx.DB(), "resource", created.Code, "", 100); err != nil {
		t.Fatalf("按资源编码查询审计失败: %v", err)
	} else {
		for _, en := range entries {
			if en.Action == "resource.archive" {
				t.Fatalf("归档事件错挂到资源编码 %s 下，应使用资源 ID %s", created.Code, created.ID)
			}
		}
	}
}
