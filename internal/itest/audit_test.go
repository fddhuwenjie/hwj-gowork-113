package itest

import (
	"testing"
	"time"

	"germplasm/internal/service"
)

// TestOutboundApproveAuditAttribution 锁定审批审计的实体归属协议：
// outbound.approve 必须归属于被审批的出库申请本身（entity_type=outbound,
// entity_id=出库申请 ID），不得改挂到批次。此前 Writer 会依据 action 把
// entity_type 改写为 batch，且 Approve 误传 o.BatchID，导致按出库申请
// 查不到该审计、按批次反而出现。
func TestOutboundApproveAuditAttribution(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	chamber := "C20"
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(500, 250, chamber)
	e.setupSensors(chamber, 2)

	o, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo:      e.unique("OUT"),
		AccessionID:    acc.ID,
		BatchID:        batch.ID,
		Qty:            100,
		Purpose:        "复壮",
		RuleVersionID:  rule.ID,
		Deadline:       e.clk.Now().Add(24 * time.Hour).Format(time.RFC3339Nano),
		IdempotencyKey: e.unique("idem"),
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	if _, err := e.svc.Outbound.Approve(ctx, "tester", o.ID, o.Version); err != nil {
		t.Fatalf("出库审批失败: %v", err)
	}

	// 按出库申请查询审计：必须能查到本次 outbound.approve。
	reqEntries, _, err := e.svc.Audit.List(ctx, e.db, "outbound", o.ID, "", 50)
	if err != nil {
		t.Fatalf("按出库申请查询审计失败: %v", err)
	}
	var foundApprove bool
	for _, en := range reqEntries {
		if en.Action == "outbound.approve" {
			foundApprove = true
			if en.EntityType != "outbound" || en.EntityID != o.ID {
				t.Fatalf("outbound.approve 审计归属错误: type=%s id=%s，期望 outbound/%s",
					en.EntityType, en.EntityID, o.ID)
			}
		}
	}
	if !foundApprove {
		t.Fatalf("按出库申请查询不到 outbound.approve 审计，记录: %+v", reqEntries)
	}

	// 按批次查询审计：审批审计不得出现在批次名下。
	batchEntries, _, err := e.svc.Audit.List(ctx, e.db, "batch", batch.ID, "", 50)
	if err != nil {
		t.Fatalf("按批次查询审计失败: %v", err)
	}
	for _, en := range batchEntries {
		if en.Action == "outbound.approve" {
			t.Fatalf("outbound.approve 审计错误归属于批次: %+v", en)
		}
	}
}
