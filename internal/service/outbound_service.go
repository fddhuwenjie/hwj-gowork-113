package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"germplasm/internal/apperr"
	"germplasm/internal/domain"
	"germplasm/internal/repository"
)

// OutboundService 负责出库申请全生命周期：创建、审批（冻结）、驳回、取消、出库。
type OutboundService struct {
	base baseService
}

// CreateOutboundInput 出库申请入参。
type CreateOutboundInput struct {
	RequestNo      string `json:"request_no"`
	AccessionID    string `json:"accession_id"`
	BatchID        string `json:"batch_id"`
	Qty            int64  `json:"qty"`
	Purpose        string `json:"purpose"`
	BreedingTarget string `json:"breeding_target"`
	RuleVersionID  string `json:"rule_version_id"`
	Deadline       string `json:"deadline"` // RFC3339
	IdempotencyKey string `json:"idempotency_key"`
}

// Create 创建出库申请（PENDING）。重复提交凭幂等键返回首个申请。
func (s *OutboundService) Create(ctx context.Context, actor string, in CreateOutboundInput) (*domain.OutboundRequest, error) {
	if err := requireNonEmpty("申请编号", in.RequestNo); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("批次", in.BatchID); err != nil {
		return nil, err
	}
	if err := requirePositive("出库数量", in.Qty); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("保存规则版本", in.RuleVersionID); err != nil {
		return nil, err
	}
	deadline, err := time.Parse(time.RFC3339Nano, in.Deadline)
	if err != nil {
		return nil, apperr.Validation("deadline 必须为 RFC3339 时间")
	}
	now := s.base.now()
	reqHash := repository.HashRequest(in)
	o := &domain.OutboundRequest{
		ID:             domain.NewID(domain.PrefixOutbound),
		RequestNo:      in.RequestNo,
		AccessionID:    in.AccessionID,
		BatchID:        in.BatchID,
		Qty:            in.Qty,
		Purpose:        in.Purpose,
		BreedingTarget: in.BreedingTarget,
		RuleVersionID:  in.RuleVersionID,
		Deadline:       deadline.UTC(),
		Status:         domain.OutboundPending,
		IdempotencyKey: in.IdempotencyKey,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	err = s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		if id, found, err := s.base.idemLookup(ctx, tx, in.IdempotencyKey, "outbound.create", reqHash); err != nil {
			return err
		} else if found {
			o.ID = id
			return errReplay
		}
		b, err := s.base.repos.Batches.Get(ctx, tx, in.BatchID)
		if err != nil {
			return err
		}
		if b.AccessionID != in.AccessionID {
			return apperr.Validation("批次不属于指定 accession")
		}
		if b.Status != domain.BatchActive {
			return apperr.Statef("批次状态 %s 不允许出库", b.Status)
		}
		if b.QtyAvailable < in.Qty {
			return apperr.Quantityf("批次可用量 %d 小于出库数量 %d", b.QtyAvailable, in.Qty)
		}
		if _, err := s.base.repos.Rules.Get(ctx, tx, in.RuleVersionID); err != nil {
			return err
		}
		if err := s.base.repos.Outbound.Insert(ctx, tx, o); err != nil {
			return err
		}
		if err := s.base.idemSave(ctx, tx, in.IdempotencyKey, "outbound.create", reqHash, o.ID); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "outbound.create", "outbound", o.ID, o, now)
	})
	if err != nil {
		if err == errReplay {
			return s.base.repos.Outbound.Get(ctx, s.base.tx.DB(), o.ID)
		}
		return nil, err
	}
	return o, nil
}

// errReplay 标记幂等重放，事务提前成功返回。
var errReplay = fmt.Errorf("幂等重放")

// freezeSampleSuffix 为拆分样本生成短唯一后缀，避免重复拆分编号冲突。
func freezeSampleSuffix() string {
	id := domain.NewID("x")
	if len(id) > 8 {
		return id[len(id)-8:]
	}
	return id
}

// Approve 审批出库申请：在同一事务内冻结样本数量、库位、保存规则与繁育目标，
// 并校验冷库环境在出库前窗口内的温湿度覆盖。任何一步失败整体回滚。
func (s *OutboundService) Approve(ctx context.Context, actor, id string, expectedVersion int64) (*domain.OutboundRequest, error) {
	now := s.base.now()
	var o *domain.OutboundRequest
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		o, err = s.base.repos.Outbound.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if o.Version != expectedVersion {
			return apperr.OptimisticLock("出库申请", id)
		}
		if err := domain.MustTransition("outbound", string(o.Status), string(domain.OutboundApproved)); err != nil {
			return err
		}
		rule, err := s.base.repos.Rules.Get(ctx, tx, o.RuleVersionID)
		if err != nil {
			return err
		}
		if rule.Status != domain.RuleActive {
			return apperr.Statef("保存规则 %s 版本 %d 未启用，不能用于出库审批", rule.Code, rule.VersionNo)
		}
		b, err := s.base.repos.Batches.Get(ctx, tx, o.BatchID)
		if err != nil {
			return err
		}
		if b.QtyAvailable < o.Qty {
			return apperr.Quantityf("批次可用量 %d 小于出库数量 %d", b.QtyAvailable, o.Qty)
		}
		// 规划冻结：FIFO 选择样本，必要时拆分样本，数量守恒。
		samples, err := s.base.repos.Samples.ListByBatch(ctx, tx, b.ID, domain.SampleInStock)
		if err != nil {
			return err
		}
		plan, err := domain.PlanFreeze(samples, o.Qty)
		if err != nil {
			return err
		}
		// 环境窗口校验：出库前 window_before_hours 内温湿度必须全覆盖且达标。
		chambers := map[string]bool{}
		for _, p := range plan {
			if p.Sample.LocationID == "" {
				return apperr.Validationf("样本 %s 尚未分配库位，不能出库", p.Sample.SampleNo)
			}
			loc, err := s.base.repos.Locations.Get(ctx, tx, p.Sample.LocationID)
			if err != nil {
				return err
			}
			chambers[loc.Chamber] = true
		}
		for chamber := range chambers {
			report, err := s.evaluateWindow(ctx, tx, rule, chamber, now, rule.WindowBeforeHours, 0)
			if err != nil {
				return err
			}
			if !report.OK {
				return apperr.Window(fmt.Sprintf("冷库 %s 出库前环境窗口覆盖不足或存在越限：覆盖率 %.2f，越限读数 %d",
					chamber, report.Coverage, report.OutOfRangeCount)).WithDetails(report)
			}
		}
		// 执行冻结：拆分样本、写入冻结明细、更新批次数量。
		for _, p := range plan {
			freezeSample := p.Sample
			if p.TakeQty < p.Sample.Qty {
				// 拆分：原样本保留剩余数量，新样本承载冻结数量。
				remainder := p.Sample
				remainder.Qty = p.Sample.Qty - p.TakeQty
				if err := s.base.repos.Samples.Update(ctx, tx, &remainder, p.Sample.Version, now); err != nil {
					return err
				}
				freezeSample = domain.Sample{
					ID:         domain.NewID(domain.PrefixSample),
					BatchID:    p.Sample.BatchID,
					SampleNo:   p.Sample.SampleNo + "-F" + freezeSampleSuffix(),
					Qty:        p.TakeQty,
					Status:     domain.SampleFrozen,
					LocationID: p.Sample.LocationID,
					Version:    1,
					CreatedAt:  now,
					UpdatedAt:  now,
				}
				if err := s.base.repos.Samples.Insert(ctx, tx, &freezeSample); err != nil {
					return err
				}
			} else {
				freezeSample.Status = domain.SampleFrozen
				if err := s.base.repos.Samples.Update(ctx, tx, &freezeSample, p.Sample.Version, now); err != nil {
					return err
				}
			}
			freeze := domain.OutboundFreeze{
				ID:         domain.NewID(domain.PrefixFreeze),
				RequestID:  o.ID,
				SampleID:   freezeSample.ID,
				LocationID: freezeSample.LocationID,
				Qty:        p.TakeQty,
				Status:     domain.FreezeActive,
				CreatedAt:  now,
			}
			if err := s.base.repos.Outbound.InsertFreeze(ctx, tx, &freeze); err != nil {
				return err
			}
		}
		b.QtyAvailable -= o.Qty
		b.QtyFrozen += o.Qty
		if err := s.base.repos.Batches.UpdateQuantities(ctx, tx, b, b.Version, now); err != nil {
			return err
		}
		if err := s.base.repos.Outbound.UpdateStatus(ctx, tx, o.ID, domain.OutboundApproved, o.Version, now); err != nil {
			return err
		}
		o.Status = domain.OutboundApproved
		o.Version++
		if err := s.base.repos.Snapshots.Add(ctx, tx, "outbound", o.ID, "APPROVED", map[string]any{
			"request": o, "batch": b, "rule": rule,
		}, now); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "outbound.approve", "outbound", o.ID, o, now)
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

// Reject 驳回待审批申请。
func (s *OutboundService) Reject(ctx context.Context, actor, id string, expectedVersion int64) (*domain.OutboundRequest, error) {
	return s.transitionSimple(ctx, actor, id, expectedVersion, domain.OutboundRejected, "outbound.reject")
}

// Cancel 取消申请：已审批申请取消时在同一事务内释放全部冻结。
func (s *OutboundService) Cancel(ctx context.Context, actor, id string, expectedVersion int64) (*domain.OutboundRequest, error) {
	now := s.base.now()
	var o *domain.OutboundRequest
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		o, err = s.base.repos.Outbound.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if o.Version != expectedVersion {
			return apperr.OptimisticLock("出库申请", id)
		}
		if o.Status != domain.OutboundRejected {
			if err := domain.MustTransition("outbound", string(o.Status), string(domain.OutboundCancelled)); err != nil {
				return err
			}
		}
		if o.Status == domain.OutboundApproved {
			if err := s.releaseFreezes(ctx, tx, o, now); err != nil {
				return err
			}
		}
		if err := s.base.repos.Outbound.UpdateStatus(ctx, tx, o.ID, domain.OutboundCancelled, o.Version, now); err != nil {
			return err
		}
		o.Status = domain.OutboundCancelled
		o.Version++
		return s.base.audit.Log(ctx, tx, actor, "outbound.cancel", "outbound", o.ID, o, now)
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

// Fulfill 执行出库：冻结样本转为出库，批次数量冻结转为已出库，
// 并校验审批后持续监控窗口的环境覆盖。事务失败整体回滚。
func (s *OutboundService) Fulfill(ctx context.Context, actor, id string, expectedVersion int64) (*domain.OutboundRequest, error) {
	now := s.base.now()
	var o *domain.OutboundRequest
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		o, err = s.base.repos.Outbound.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if o.Version != expectedVersion {
			return apperr.OptimisticLock("出库申请", id)
		}
		if err := domain.MustTransition("outbound", string(o.Status), string(domain.OutboundFulfilled)); err != nil {
			return err
		}
		rule, err := s.base.repos.Rules.Get(ctx, tx, o.RuleVersionID)
		if err != nil {
			return err
		}
		// 出库后窗口：审批时刻起到现在（不超过 window_after_hours）环境必须持续受监控。
		freezes, err := s.base.repos.Outbound.ListFreezes(ctx, tx, o.ID, domain.FreezeActive)
		if err != nil {
			return err
		}
		if len(freezes) == 0 {
			return apperr.Statef("出库申请 %s 无有效冻结明细", o.RequestNo)
		}
		chambers := map[string]bool{}
		for _, f := range freezes {
			loc, err := s.base.repos.Locations.Get(ctx, tx, f.LocationID)
			if err != nil {
				return err
			}
			chambers[loc.Chamber] = true
		}
		// 出库后窗口以审批时间（快照于 updated_at 之前版本）近似为申请最近更新时刻。
		windowHours := rule.WindowAfterHours
		if elapsed := int(now.Sub(o.UpdatedAt).Hours()); elapsed < windowHours {
			windowHours = elapsed
		}
		if windowHours > 0 {
			for chamber := range chambers {
				report, err := s.evaluateWindow(ctx, tx, rule, chamber, now, windowHours, 0)
				if err != nil {
					return err
				}
				if !report.OK {
					return apperr.Window(fmt.Sprintf("冷库 %s 出库后环境窗口覆盖不足或存在越限：覆盖率 %.2f，越限读数 %d",
						chamber, report.Coverage, report.OutOfRangeCount)).WithDetails(report)
				}
			}
		}
		b, err := s.base.repos.Batches.Get(ctx, tx, o.BatchID)
		if err != nil {
			return err
		}
		for _, f := range freezes {
			smp, err := s.base.repos.Samples.Get(ctx, tx, f.SampleID)
			if err != nil {
				return err
			}
			smp.Status = domain.SampleOutbound
			if err := s.base.repos.Samples.Update(ctx, tx, smp, smp.Version, now); err != nil {
				return err
			}
			if err := s.base.repos.Outbound.UpdateFreezeStatus(ctx, tx, f.ID, domain.FreezeConsumed); err != nil {
				return err
			}
		}
		b.QtyFrozen -= o.Qty
		b.QtyOutbound += o.Qty
		if b.QtyAvailable == 0 && b.QtyFrozen == 0 && b.Status == domain.BatchActive {
			b.Status = domain.BatchExhausted
		}
		if err := s.base.repos.Batches.UpdateQuantities(ctx, tx, b, b.Version, now); err != nil {
			return err
		}
		if err := s.base.repos.Outbound.UpdateStatus(ctx, tx, o.ID, domain.OutboundFulfilled, o.Version, now); err != nil {
			return err
		}
		o.Status = domain.OutboundFulfilled
		o.Version++
		if err := s.base.repos.Snapshots.Add(ctx, tx, "outbound", o.ID, "FULFILLED", map[string]any{
			"request": o, "batch": b,
		}, now); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "outbound.fulfill", "outbound", o.ID, o, now)
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

// transitionSimple 处理无附带动作的状态转换（驳回）。
func (s *OutboundService) transitionSimple(ctx context.Context, actor, id string, expectedVersion int64, to domain.OutboundStatus, action string) (*domain.OutboundRequest, error) {
	now := s.base.now()
	var o *domain.OutboundRequest
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		o, err = s.base.repos.Outbound.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if o.Version != expectedVersion {
			return apperr.OptimisticLock("出库申请", id)
		}
		if err := domain.MustTransition("outbound", string(o.Status), string(to)); err != nil {
			return err
		}
		if err := s.base.repos.Outbound.UpdateStatus(ctx, tx, o.ID, to, o.Version, now); err != nil {
			return err
		}
		o.Status = to
		o.Version++
		return s.base.audit.Log(ctx, tx, actor, action, "outbound", o.ID, o, now)
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

// releaseFreezes 释放申请的全部冻结：样本回到在库，批次数量回补。
func (s *OutboundService) releaseFreezes(ctx context.Context, tx *sql.Tx, o *domain.OutboundRequest, now time.Time) error {
	freezes, err := s.base.repos.Outbound.ListFreezes(ctx, tx, o.ID, domain.FreezeActive)
	if err != nil {
		return err
	}
	b, err := s.base.repos.Batches.Get(ctx, tx, o.BatchID)
	if err != nil {
		return err
	}
	for _, f := range freezes {
		smp, err := s.base.repos.Samples.Get(ctx, tx, f.SampleID)
		if err != nil {
			return err
		}
		smp.Status = domain.SampleInStock
		if err := s.base.repos.Samples.Update(ctx, tx, smp, smp.Version, now); err != nil {
			return err
		}
		if err := s.base.repos.Outbound.UpdateFreezeStatus(ctx, tx, f.ID, domain.FreezeReleased); err != nil {
			return err
		}
	}
	b.QtyFrozen -= o.Qty
	b.QtyAvailable += o.Qty
	return s.base.repos.Batches.UpdateQuantities(ctx, tx, b, b.Version, now)
}

// evaluateWindow 评估冷库环境窗口覆盖。
func (s *OutboundService) evaluateWindow(ctx context.Context, tx *sql.Tx, rule *domain.RuleVersion, chamber string, eventAt time.Time, beforeHours, afterHours int) (domain.EnvWindowReport, error) {
	start := eventAt.Add(-time.Duration(beforeHours) * time.Hour)
	end := eventAt.Add(time.Duration(afterHours) * time.Hour)
	tempReadings, err := s.base.repos.Sensors.ReadingsInWindow(ctx, tx, chamber, domain.MetricTemperature, start, end)
	if err != nil {
		return domain.EnvWindowReport{}, err
	}
	humReadings, err := s.base.repos.Sensors.ReadingsInWindow(ctx, tx, chamber, domain.MetricHumidity, start, end)
	if err != nil {
		return domain.EnvWindowReport{}, err
	}
	evalRule := *rule
	evalRule.WindowBeforeHours = beforeHours
	evalRule.WindowAfterHours = afterHours
	return domain.EvaluateEnvWindow(&evalRule, eventAt, tempReadings, humReadings), nil
}

// Get 查询出库申请详情。
func (s *OutboundService) Get(ctx context.Context, id string) (*domain.OutboundRequest, error) {
	return s.base.repos.Outbound.Get(ctx, s.base.tx.DB(), id)
}

// List 分页查询出库申请。
func (s *OutboundService) List(ctx context.Context, status, cursor string, limit int) (*repository.Page[domain.OutboundRequest], error) {
	return s.base.repos.Outbound.List(ctx, s.base.tx.DB(), status, cursor, repository.NormalizeLimit(limit))
}

// ListFreezes 查询申请冻结明细。
func (s *OutboundService) ListFreezes(ctx context.Context, id string) ([]domain.OutboundFreeze, error) {
	if _, err := s.base.repos.Outbound.Get(ctx, s.base.tx.DB(), id); err != nil {
		return nil, err
	}
	return s.base.repos.Outbound.ListFreezes(ctx, s.base.tx.DB(), id, "")
}
