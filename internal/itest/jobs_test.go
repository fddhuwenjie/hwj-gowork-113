package itest

import (
	"errors"
	"testing"
	"time"

	"germplasm/internal/domain"
	"germplasm/internal/service"
)

// TestBreedingTimeoutJob 繁育超时作业：超期 ACTIVE 计划标记 TIMEOUT 并产生告警。
func TestBreedingTimeoutJob(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C16")
	// 推进时间超过繁育期限
	e.clk.Advance(31 * 24 * time.Hour)
	e.sched.RunOnceForTest(ctx)
	p, err := e.svc.Breeding.GetPlan(ctx, plan.ID)
	if err != nil {
		t.Fatalf("查询计划失败: %v", err)
	}
	if p.Status != domain.PlanTimeout {
		t.Fatalf("超期计划应标记 TIMEOUT，实际 %s", p.Status)
	}
	page, err := e.svc.Repos.Alerts.List(ctx, e.db, "OPEN", domain.AlertBreedingTimeout, "", 50)
	if err != nil {
		t.Fatalf("查询告警失败: %v", err)
	}
	found := false
	for _, a := range page.Items {
		if a.RefID == plan.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("应产生繁育超时告警")
	}
}

// TestOutboundDueSoonJob 出库临期作业产生告警且不重复。
func TestOutboundDueSoonJob(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	rule := e.setupRule()
	_, acc, batch, _ := e.setupBase(500, 250, "C17")
	e.setupSensors("C17", 2)
	o, err := e.svc.Outbound.Create(ctx, "tester", service.CreateOutboundInput{
		RequestNo: e.unique("OUT"), AccessionID: acc.ID, BatchID: batch.ID, Qty: 100,
		RuleVersionID: rule.ID, Deadline: e.clk.Now().Add(12 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("创建出库申请失败: %v", err)
	}
	if _, err := e.svc.Outbound.Approve(ctx, "tester", o.ID, o.Version); err != nil {
		t.Fatalf("审批失败: %v", err)
	}
	e.sched.RunOnceForTest(ctx)
	e.sched.RunOnceForTest(ctx) // 再次扫描不应重复告警
	page, err := e.svc.Repos.Alerts.List(ctx, e.db, "OPEN", domain.AlertOutboundDueSoon, "", 50)
	if err != nil {
		t.Fatalf("查询告警失败: %v", err)
	}
	count := 0
	for _, a := range page.Items {
		if a.RefID == o.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("临期告警应恰好 1 条，实际 %d", count)
	}
}

// TestEnvAlertJob 环境告警作业：越限读数产生告警。
func TestEnvAlertJob(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	e.setupRule()
	e.setupSensorsWithValue("C18", 0, -5, 30) // 温度越限
	e.sched.RunOnceForTest(ctx)
	page, err := e.svc.Repos.Alerts.List(ctx, e.db, "OPEN", domain.AlertEnvOutOfRange, "", 50)
	if err != nil {
		t.Fatalf("查询告警失败: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatalf("应产生环境越限告警")
	}
}

// TestJobRetryAndRecovery 失败作业按退避重试，超过最大次数进入 FAILED；
// 重启后 RUNNING 作业被恢复为 PENDING 并可继续执行。
func TestJobRetryAndRecovery(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	now := e.clk.Now()
	// 构造一个未知类型作业，借 Fail 路径验证重试语义。
	j := &domain.Job{
		ID: domain.NewID(domain.PrefixJob), Type: "unknown_type", Payload: "{}",
		Status: domain.JobPending, MaxAttempts: 2, NextRunAt: now, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := e.svc.Repos.Jobs.Enqueue(ctx, e.db, j); err != nil {
		t.Fatalf("入队失败: %v", err)
	}
	claimed, err := e.svc.Repos.Jobs.Claim(ctx, e.db, now)
	if err != nil || claimed == nil {
		t.Fatalf("抢占作业失败: %v", err)
	}
	cause := errors.New("模拟失败")
	// 第一次失败：重排为 PENDING，退避 1s
	if err := e.svc.Repos.Jobs.Fail(ctx, e.db, claimed, cause, now); err != nil {
		t.Fatalf("标记失败出错: %v", err)
	}
	after, _ := e.svc.Repos.Jobs.Get(ctx, e.db, j.ID)
	if after.Status != domain.JobPending || after.Attempts != 1 {
		t.Fatalf("首次失败后应重新排队: %+v", after)
	}
	if !after.NextRunAt.After(now) {
		t.Fatalf("退避后下次运行时间应推迟")
	}
	// 推进时钟再次执行并失败：达到最大次数进入 FAILED
	e.clk.Advance(2 * time.Second)
	claimed2, err := e.svc.Repos.Jobs.Claim(ctx, e.db, e.clk.Now())
	if err != nil || claimed2 == nil {
		t.Fatalf("再次抢占失败: %v", err)
	}
	if err := e.svc.Repos.Jobs.Fail(ctx, e.db, claimed2, cause, e.clk.Now()); err != nil {
		t.Fatalf("标记失败出错: %v", err)
	}
	final, _ := e.svc.Repos.Jobs.Get(ctx, e.db, j.ID)
	if final.Status != domain.JobFailed {
		t.Fatalf("达到最大尝试次数应为 FAILED，实际 %s", final.Status)
	}
	// 重启恢复：构造 RUNNING 作业，reopen 后 Recover 重置为 PENDING。
	stuck := &domain.Job{
		ID: domain.NewID(domain.PrefixJob), Type: domain.JobEnvAlertScan, Payload: "{}",
		Status: domain.JobRunning, MaxAttempts: 3, NextRunAt: e.clk.Now(), Version: 1,
		CreatedAt: e.clk.Now(), UpdatedAt: e.clk.Now(),
	}
	if err := e.svc.Repos.Jobs.Enqueue(ctx, e.db, stuck); err != nil {
		t.Fatalf("入队失败: %v", err)
	}
	e.reopen()
	if err := e.sched.Recover(ctx); err != nil {
		t.Fatalf("重启恢复失败: %v", err)
	}
	recovered, err := e.svc.Repos.Jobs.Get(ctx, e.db, stuck.ID)
	if err != nil {
		t.Fatalf("重启后作业丢失: %v", err)
	}
	if recovered.Status != domain.JobPending {
		t.Fatalf("重启恢复后应为 PENDING，实际 %s", recovered.Status)
	}
	// 重启后调度可继续执行该作业。
	e.sched.RunOnceForTest(ctx)
	done, _ := e.svc.Repos.Jobs.Get(ctx, e.db, stuck.ID)
	if done.Status != domain.JobDone {
		t.Fatalf("恢复的作业应被执行完成，实际 %s", done.Status)
	}
}

// TestRestockPendingJob 回存验收超期作业产生告警。
func TestRestockPendingJob(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctx
	plan := e.breedToTest(t, "C19")
	rb, err := e.svc.Restock.Create(ctx, "tester", service.CreateRestockInput{
		RequestNo: e.unique("RST"), PlanID: plan.ID, Qty: 800,
	})
	if err != nil {
		t.Fatalf("创建回存单失败: %v", err)
	}
	e.clk.Advance(73 * time.Hour)
	e.sched.RunOnceForTest(ctx)
	page, err := e.svc.Repos.Alerts.List(ctx, e.db, "OPEN", domain.AlertRestockPending, "", 50)
	if err != nil {
		t.Fatalf("查询告警失败: %v", err)
	}
	found := false
	for _, a := range page.Items {
		if a.RefID == rb.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("应产生回存验收超期告警")
	}
}
