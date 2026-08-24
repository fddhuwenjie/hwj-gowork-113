package service

import (
	"context"
	"database/sql"
	"fmt"

	"germplasm/internal/apperr"
	"germplasm/internal/domain"
	"germplasm/internal/repository"
)

// RestockService 负责回存批次验收：创建验收单、验收（建新批次）、驳回。
type RestockService struct {
	base baseService
}

// CreateRestockInput 回存验收单入参。
type CreateRestockInput struct {
	RequestNo      string `json:"request_no"`
	PlanID         string `json:"plan_id"`
	Qty            int64  `json:"qty"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Create 创建回存验收单（PENDING）。重复提交凭幂等键返回首单。
func (s *RestockService) Create(ctx context.Context, actor string, in CreateRestockInput) (*domain.RestockBatch, error) {
	if err := requireNonEmpty("回存单号", in.RequestNo); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("繁育计划", in.PlanID); err != nil {
		return nil, err
	}
	if err := requirePositive("回存数量", in.Qty); err != nil {
		return nil, err
	}
	now := s.base.now()
	reqHash := repository.HashRequest(in)
	rb := &domain.RestockBatch{
		ID:             domain.NewID(domain.PrefixRestock),
		RequestNo:      in.RequestNo,
		PlanID:         in.PlanID,
		Qty:            in.Qty,
		Status:         domain.RestockPending,
		IdempotencyKey: in.IdempotencyKey,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		if id, found, err := s.base.idemLookup(ctx, tx, in.IdempotencyKey, "restock.create", reqHash); err != nil {
			return err
		} else if found {
			rb.ID = id
			return errReplay
		}
		p, err := s.base.repos.Breeding.GetPlan(ctx, tx, in.PlanID)
		if err != nil {
			return err
		}
		if !domain.CanTransition("plan", string(p.Status), string(domain.PlanCompleted)) {
			return apperr.Statef("繁育计划状态 %s 不允许申请回存", p.Status)
		}
		if err := s.base.repos.Restock.Insert(ctx, tx, rb); err != nil {
			return err
		}
		if err := s.base.idemSave(ctx, tx, in.IdempotencyKey, "restock.create", reqHash, rb.ID); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "restock.create", "restock", rb.ID, rb, now)
	})
	if err != nil {
		if err == errReplay {
			return s.base.repos.Restock.Get(ctx, s.base.tx.DB(), rb.ID)
		}
		return nil, err
	}
	return rb, nil
}

// Accept 回存验收：在同一事务内完成质量复核、创建新批次、
// 建立谱系、完成繁育计划、关闭耗尽母批并写入历史快照，失败整体回滚。
// 检测覆盖不足或纯度不合格时不得回存为合格批次。
func (s *RestockService) Accept(ctx context.Context, actor, id string, expectedVersion int64) (*domain.RestockBatch, error) {
	now := s.base.now()
	var rb *domain.RestockBatch
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		rb, err = s.base.repos.Restock.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if rb.Version != expectedVersion {
			return apperr.OptimisticLock("回存验收单", id)
		}
		if err := domain.MustTransition("restock", string(rb.Status), string(domain.RestockAccepted)); err != nil {
			return err
		}
		plan, err := s.base.repos.Breeding.GetPlan(ctx, tx, rb.PlanID)
		if err != nil {
			return err
		}
		if err := domain.MustTransition("plan", string(plan.Status), string(domain.PlanCompleted)); err != nil {
			return err
		}
		// 质量复核：必须存在封存合格结论，且覆盖率与纯度满足规则门槛。
		sealed, err := s.base.repos.Purity.LatestSealed(ctx, tx, plan.ID)
		if err != nil {
			return err
		}
		if sealed == nil {
			return apperr.Quality("回存验收失败：计划无封存的纯度检测结论")
		}
		o, err := s.base.repos.Outbound.Get(ctx, tx, plan.OutboundRequestID)
		if err != nil {
			return err
		}
		rule, err := s.base.repos.Rules.Get(ctx, tx, o.RuleVersionID)
		if err != nil {
			return err
		}
		if sealed.Verdict != domain.VerdictPass ||
			sealed.CoverageRatio < rule.MinCoverage ||
			sealed.PurityRate < rule.MinPurity {
			return apperr.Quality(fmt.Sprintf("检测覆盖不足或纯度不合格，不得回存为合格批次：覆盖率 %.2f（门槛 %.2f），纯度 %.2f（门槛 %.2f）",
				sealed.CoverageRatio, rule.MinCoverage, sealed.PurityRate, rule.MinPurity))
		}
		// 创建新回存批次，母批与旧检测保持只读。
		mother, err := s.base.repos.Batches.Get(ctx, tx, plan.BatchID)
		if err != nil {
			return err
		}
		acc, err := s.base.repos.Resources.GetAccession(ctx, tx, mother.AccessionID)
		if err != nil {
			return err
		}
		newBatch := &domain.Batch{
			ID:            domain.NewID(domain.PrefixBatch),
			AccessionID:   mother.AccessionID,
			BatchNo:       fmt.Sprintf("%s-RS", rb.RequestNo),
			Kind:          domain.BatchRestock,
			MotherBatchID: mother.ID,
			Unit:          mother.Unit,
			QtyTotal:      rb.Qty,
			QtyAvailable:  rb.Qty,
			Status:        domain.BatchActive,
			Version:       1,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := s.base.repos.Batches.Insert(ctx, tx, newBatch); err != nil {
			return err
		}
		// 谱系关联：母批 -> 回存批次。
		edge := &domain.LineageEdge{
			ID:            domain.NewID(domain.PrefixLineage),
			ResourceID:    acc.ResourceID,
			ParentBatchID: mother.ID,
			ChildBatchID:  newBatch.ID,
			Relation:      "RESTOCK",
			CreatedAt:     now,
		}
		if err := s.base.repos.Lineage.InsertEdge(ctx, tx, edge); err != nil {
			return err
		}
		// 母批耗尽则关闭。
		if mother.QtyAvailable == 0 && mother.QtyFrozen == 0 && mother.Status != domain.BatchClosed {
			mother.Status = domain.BatchClosed
			mother.ClosedAt = &now
			if err := s.base.repos.Batches.UpdateQuantities(ctx, tx, mother, mother.Version, now); err != nil {
				return err
			}
		}
		// 繁育计划完成。
		if err := s.base.repos.Breeding.UpdatePlanStatus(ctx, tx, plan.ID, domain.PlanCompleted, plan.Version, now); err != nil {
			return err
		}
		if err := s.base.repos.Restock.UpdateStatus(ctx, tx, rb.ID, domain.RestockAccepted, newBatch.ID, "", rb.Version, now); err != nil {
			return err
		}
		rb.Status = domain.RestockAccepted
		rb.NewBatchID = newBatch.ID
		rb.Version++
		// 历史快照：回存验收事件保留全部关键实体状态。
		if err := s.base.repos.Snapshots.Add(ctx, tx, "restock", rb.ID, "ACCEPTED", map[string]any{
			"restock": rb, "new_batch": newBatch, "mother_batch": mother, "sealed_test": sealed, "plan": plan,
		}, now); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "restock.accept", "restock", rb.ID, rb, now)
	})
	if err != nil {
		return nil, err
	}
	return rb, nil
}

// Reject 驳回回存验收单（必须给出原因）。
func (s *RestockService) Reject(ctx context.Context, actor, id, reason string, expectedVersion int64) (*domain.RestockBatch, error) {
	if err := requireNonEmpty("驳回原因", reason); err != nil {
		return nil, err
	}
	now := s.base.now()
	var rb *domain.RestockBatch
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		rb, err = s.base.repos.Restock.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if rb.Version != expectedVersion {
			return apperr.OptimisticLock("回存验收单", id)
		}
		if err := domain.MustTransition("restock", string(rb.Status), string(domain.RestockRejected)); err != nil {
			return err
		}
		if err := s.base.repos.Restock.UpdateStatus(ctx, tx, rb.ID, domain.RestockRejected, "", reason, rb.Version, now); err != nil {
			return err
		}
		rb.Status = domain.RestockRejected
		rb.RejectReason = reason
		rb.Version++
		if err := s.base.repos.Snapshots.Add(ctx, tx, "restock", rb.ID, "REJECTED", rb, now); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "restock.reject", "restock", rb.ID, rb, now)
	})
	if err != nil {
		return nil, err
	}
	return rb, nil
}

// Get 查询回存验收单详情。
func (s *RestockService) Get(ctx context.Context, id string) (*domain.RestockBatch, error) {
	return s.base.repos.Restock.Get(ctx, s.base.tx.DB(), id)
}

// List 分页查询回存验收单。
func (s *RestockService) List(ctx context.Context, status, cursor string, limit int) (*repository.Page[domain.RestockBatch], error) {
	return s.base.repos.Restock.List(ctx, s.base.tx.DB(), status, cursor, repository.NormalizeLimit(limit))
}
