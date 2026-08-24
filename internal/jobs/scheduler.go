// Package jobs 提供可重启恢复的持久化后台作业调度。
package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"germplasm/internal/clock"
	"germplasm/internal/config"
	"germplasm/internal/domain"
	"germplasm/internal/service"
)

// Scheduler 轮询作业表执行到期作业，并周期入队扫描类作业。
// 作业持久化在 SQLite 中，进程重启后自动恢复执行。
type Scheduler struct {
	svc    *service.Services
	cfg    config.Config
	clk    clock.Clock
	log    *slog.Logger
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewScheduler 创建调度器。
func NewScheduler(svc *service.Services, cfg config.Config, clk clock.Clock, log *slog.Logger) *Scheduler {
	return &Scheduler{svc: svc, cfg: cfg, clk: clk, log: log}
}

// Recover 启动前恢复：崩溃遗留的 RUNNING 作业重置为 PENDING。
func (s *Scheduler) Recover(ctx context.Context) error {
	n, err := s.svc.Repos.Jobs.RecoverStuck(ctx, s.svc.Tx.DB(), s.clk.Now())
	if err != nil {
		return err
	}
	if n > 0 {
		s.log.Info("恢复中断作业", "count", n)
	}
	return nil
}

// Start 启动调度循环与周期扫描入队循环。
func (s *Scheduler) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(2)
	go s.pollLoop(ctx)
	go s.scanLoop(ctx)
}

// Stop 停止调度器并等待循环退出。
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// pollLoop 按间隔抢占并执行到期作业。
func (s *Scheduler) pollLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.cfg.JobInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

// scanLoop 周期入队扫描类作业（去重），保证重启后扫描任务持续存在。
func (s *Scheduler) scanLoop(ctx context.Context) {
	defer s.wg.Done()
	s.enqueueScans(ctx)
	ticker := time.NewTicker(s.cfg.JobScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.enqueueScans(ctx)
		}
	}
}

// enqueueScans 入队全部周期扫描作业。
func (s *Scheduler) enqueueScans(ctx context.Context) {
	for _, typ := range []string{
		domain.JobEnvAlertScan,
		domain.JobOutboundDueScan,
		domain.JobBreedingTimeoutScan,
		domain.JobRestockPendingScan,
	} {
		now := s.clk.Now()
		j := &domain.Job{
			ID:          domain.NewID(domain.PrefixJob),
			Type:        typ,
			Payload:     "{}",
			Status:      domain.JobPending,
			MaxAttempts: s.cfg.JobMaxAttempts,
			NextRunAt:   now,
			Version:     1,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.svc.Repos.Jobs.EnqueueIfAbsent(ctx, s.svc.Tx.DB(), j); err != nil {
			s.log.Error("入队扫描作业失败", "type", typ, "error", err)
		}
	}
}

// runOnce 抢占并执行一个到期作业，失败按指数退避重试。
func (s *Scheduler) runOnce(ctx context.Context) {
	j, err := s.svc.Repos.Jobs.Claim(ctx, s.svc.Tx.DB(), s.clk.Now())
	if err != nil {
		s.log.Error("抢占作业失败", "error", err)
		return
	}
	if j == nil {
		return
	}
	if err := s.execute(ctx, j); err != nil {
		s.log.Warn("作业执行失败，将按退避重试", "job", j.ID, "type", j.Type, "attempts", j.Attempts, "error", err)
		if ferr := s.svc.Repos.Jobs.Fail(ctx, s.svc.Tx.DB(), j, err, s.clk.Now()); ferr != nil {
			s.log.Error("标记作业失败状态出错", "job", j.ID, "error", ferr)
		}
		return
	}
	if err := s.svc.Repos.Jobs.Complete(ctx, s.svc.Tx.DB(), j.ID, s.clk.Now()); err != nil {
		s.log.Error("标记作业完成出错", "job", j.ID, "error", err)
	}
}

// execute 按作业类型分发执行。
func (s *Scheduler) execute(ctx context.Context, j *domain.Job) error {
	h := &Handlers{svc: s.svc, cfg: s.cfg, clk: s.clk}
	switch j.Type {
	case domain.JobEnvAlertScan:
		return h.EnvAlertScan(ctx)
	case domain.JobOutboundDueScan:
		return h.OutboundDueScan(ctx)
	case domain.JobBreedingTimeoutScan:
		return h.BreedingTimeoutScan(ctx)
	case domain.JobRestockPendingScan:
		return h.RestockPendingScan(ctx)
	default:
		return nil // 未知类型直接完成，避免毒消息阻塞队列
	}
}

// RunOnceForTest 暴露单次调度执行，供测试驱动假时钟场景。
func (s *Scheduler) RunOnceForTest(ctx context.Context) {
	s.enqueueScans(ctx)
	for i := 0; i < 8; i++ {
		s.runOnce(ctx)
	}
}
