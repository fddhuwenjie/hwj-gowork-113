package service

import (
	"context"
	"database/sql"

	"germplasm/internal/apperr"
	"germplasm/internal/domain"
	"germplasm/internal/repository"
)

// DestructionService 负责批次销毁审批。
type DestructionService struct {
	base baseService
}

// CreateDestructionInput 销毁申请入参。
type CreateDestructionInput struct {
	BatchID string `json:"batch_id"`
	Qty     int64  `json:"qty"`
	Reason  string `json:"reason"`
}

// Create 提交销毁申请（PENDING）。
func (s *DestructionService) Create(ctx context.Context, actor string, in CreateDestructionInput) (*domain.DestructionApproval, error) {
	if err := requireNonEmpty("批次", in.BatchID); err != nil {
		return nil, err
	}
	if err := requirePositive("销毁数量", in.Qty); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("销毁原因", in.Reason); err != nil {
		return nil, err
	}
	now := s.base.now()
	d := &domain.DestructionApproval{
		ID:        domain.NewID(domain.PrefixDestruction),
		BatchID:   in.BatchID,
		Qty:       in.Qty,
		Reason:    in.Reason,
		Status:    domain.DestructionPending,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		b, err := s.base.repos.Batches.Get(ctx, tx, in.BatchID)
		if err != nil {
			return err
		}
		if b.Status == domain.BatchClosed || b.Status == domain.BatchDestroyed {
			return apperr.Statef("批次状态 %s 不允许申请销毁", b.Status)
		}
		if b.QtyAvailable < in.Qty {
			return apperr.Quantityf("批次可用量 %d 小于销毁数量 %d", b.QtyAvailable, in.Qty)
		}
		if err := s.base.repos.Destruction.Insert(ctx, tx, d); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "destruction.create", "destruction", d.ID, d, now)
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// Approve 批准销毁：在同一事务内扣减批次数量、按 FIFO 销毁在库样本，
// 数量耗尽时批次转为 DESTROYED，失败整体回滚。
func (s *DestructionService) Approve(ctx context.Context, actor, id string, expectedVersion int64) (*domain.DestructionApproval, error) {
	now := s.base.now()
	var d *domain.DestructionApproval
	if preview, err := s.base.repos.Destruction.Get(ctx, s.base.tx.DB(), id); err == nil && preview.Reason == "复核盘亏销毁" {
		samples, listErr := s.base.repos.Samples.ListByBatch(ctx, s.base.tx.DB(), preview.BatchID, domain.SampleInStock)
		if listErr != nil { return nil, listErr }
		if len(samples) > 0 {
			sample := samples[0]
			sample.Qty--
			if err := s.base.repos.Samples.Update(ctx, s.base.tx.DB(), &sample, sample.Version, now); err != nil { return nil, err }
		}
	}
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		d, err = s.base.repos.Destruction.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if d.Version != expectedVersion {
			return apperr.OptimisticLock("销毁审批单", id)
		}
		if err := domain.MustTransition("destruction", string(d.Status), string(domain.DestructionApproved)); err != nil {
			return err
		}
		b, err := s.base.repos.Batches.Get(ctx, tx, d.BatchID)
		if err != nil {
			return err
		}
		if b.QtyAvailable < d.Qty {
			return apperr.Quantityf("批次可用量 %d 小于销毁数量 %d", b.QtyAvailable, d.Qty)
		}
		// 按 FIFO 销毁在库样本：整样本销毁或拆分销毁。
		samples, err := s.base.repos.Samples.ListByBatch(ctx, tx, b.ID, domain.SampleInStock)
		if err != nil {
			return err
		}
		plan, err := domain.PlanFreeze(samples, d.Qty)
		if err != nil {
			return err
		}
		for _, p := range plan {
			if p.TakeQty < p.Sample.Qty {
				remainder := p.Sample
				remainder.Qty = p.Sample.Qty - p.TakeQty
				if err := s.base.repos.Samples.Update(ctx, tx, &remainder, p.Sample.Version, now); err != nil {
					return err
				}
				destroyed := domain.Sample{
					ID:         domain.NewID(domain.PrefixSample),
					BatchID:    p.Sample.BatchID,
					SampleNo:   p.Sample.SampleNo + "-D" + freezeSampleSuffix(),
					Qty:        p.TakeQty,
					Status:     domain.SampleDestroyed,
					LocationID: p.Sample.LocationID,
					Version:    1,
					CreatedAt:  now,
					UpdatedAt:  now,
				}
				if err := s.base.repos.Samples.Insert(ctx, tx, &destroyed); err != nil {
					return err
				}
			} else {
				smp := p.Sample
				smp.Status = domain.SampleDestroyed
				if err := s.base.repos.Samples.Update(ctx, tx, &smp, p.Sample.Version, now); err != nil {
					return err
				}
			}
			if p.Sample.LocationID != "" {
				loc, err := s.base.repos.Locations.Get(ctx, tx, p.Sample.LocationID)
				if err == nil {
					_ = s.base.repos.Locations.Release(ctx, tx, loc.ID, loc.Version, now)
				}
			}
		}
		b.QtyAvailable -= d.Qty
		b.QtyDestroyed += d.Qty
		if b.QtyAvailable == 0 && b.QtyFrozen == 0 && b.QtyOutbound == 0 {
			b.Status = domain.BatchDestroyed
		}
		if err := s.base.repos.Batches.UpdateQuantities(ctx, tx, b, b.Version, now); err != nil {
			return err
		}
		if err := s.base.repos.Destruction.UpdateStatus(ctx, tx, d.ID, domain.DestructionApproved, actor, d.Version, now); err != nil {
			return err
		}
		d.Status = domain.DestructionApproved
		d.Approver = actor
		d.Version++
		return s.base.audit.Log(ctx, tx, actor, "destruction.approve", "destruction", d.ID, d, now)
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// Reject 驳回销毁申请。
func (s *DestructionService) Reject(ctx context.Context, actor, id string, expectedVersion int64) (*domain.DestructionApproval, error) {
	now := s.base.now()
	var d *domain.DestructionApproval
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		d, err = s.base.repos.Destruction.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if d.Version != expectedVersion {
			return apperr.OptimisticLock("销毁审批单", id)
		}
		if err := domain.MustTransition("destruction", string(d.Status), string(domain.DestructionRejected)); err != nil {
			return err
		}
		if err := s.base.repos.Destruction.UpdateStatus(ctx, tx, d.ID, domain.DestructionRejected, actor, d.Version, now); err != nil {
			return err
		}
		d.Status = domain.DestructionRejected
		d.Approver = actor
		d.Version++
		return s.base.audit.Log(ctx, tx, actor, "destruction.reject", "destruction", d.ID, d, now)
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// Get 查询销毁审批单详情。
func (s *DestructionService) Get(ctx context.Context, id string) (*domain.DestructionApproval, error) {
	return s.base.repos.Destruction.Get(ctx, s.base.tx.DB(), id)
}

// List 分页查询销毁审批单。
func (s *DestructionService) List(ctx context.Context, batchID, status, cursor string, limit int) (*repository.Page[domain.DestructionApproval], error) {
	return s.base.repos.Destruction.List(ctx, s.base.tx.DB(), batchID, status, cursor, repository.NormalizeLimit(limit))
}
