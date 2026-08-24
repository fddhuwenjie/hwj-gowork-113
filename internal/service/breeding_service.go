package service

import (
	"context"
	"database/sql"
	"time"

	"germplasm/internal/apperr"
	"germplasm/internal/domain"
	"germplasm/internal/repository"
)

// BreedingService 负责繁育计划（繁育批次）与田间观察。
type BreedingService struct {
	base baseService
}

// CreatePlanInput 繁育计划建立入参。
type CreatePlanInput struct {
	PlanNo            string `json:"plan_no"`
	OutboundRequestID string `json:"outbound_request_id"`
	TargetQty         int64  `json:"target_qty"`
	Plot              string `json:"plot"`
	Deadline          string `json:"deadline"` // RFC3339，繁育期限
}

// CreatePlan 基于已出库申请建立繁育计划；一个出库申请只能建立一个计划。
func (s *BreedingService) CreatePlan(ctx context.Context, actor string, in CreatePlanInput) (*domain.BreedingPlan, error) {
	if err := requireNonEmpty("计划编号", in.PlanNo); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("出库申请", in.OutboundRequestID); err != nil {
		return nil, err
	}
	if err := requirePositive("繁育目标数量", in.TargetQty); err != nil {
		return nil, err
	}
	deadline, err := time.Parse(time.RFC3339Nano, in.Deadline)
	if err != nil {
		return nil, apperr.Validation("deadline 必须为 RFC3339 时间")
	}
	now := s.base.now()
	p := &domain.BreedingPlan{
		ID:                domain.NewID(domain.PrefixPlan),
		PlanNo:            in.PlanNo,
		OutboundRequestID: in.OutboundRequestID,
		TargetQty:         in.TargetQty,
		Plot:              in.Plot,
		Deadline:          deadline.UTC(),
		Status:            domain.PlanActive,
		Version:           1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	err = s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		o, err := s.base.repos.Outbound.Get(ctx, tx, in.OutboundRequestID)
		if err != nil {
			return err
		}
		if o.Status != domain.OutboundFulfilled {
			return apperr.Statef("出库申请状态 %s 不允许建立繁育计划，须先完成出库", o.Status)
		}
		p.BatchID = o.BatchID
		if err := s.base.repos.Breeding.InsertPlan(ctx, tx, p); err != nil {
			return err
		}
		if err := s.base.repos.Snapshots.Add(ctx, tx, "breeding_plan", p.ID, "CREATED", map[string]any{
			"plan": p, "outbound": o,
		}, now); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "breeding.create", "breeding_plan", p.ID, p, now)
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// GetPlan 查询繁育计划详情。
// 母批 ID（BatchID）与原出库申请 ID（OutboundRequestID）在持久化层各自独立，
// 详情读取须原样返回两者，不得依据 plot 编码形式（大区/小区分层编码含 "/"）
// 串写归一，否则回存验收将无法分别追溯母批来源与出库来源。
func (s *BreedingService) GetPlan(ctx context.Context, id string) (*domain.BreedingPlan, error) {
	return s.base.repos.Breeding.GetPlan(ctx, s.base.tx.DB(), id)
}

// ListPlans 分页查询繁育计划。
func (s *BreedingService) ListPlans(ctx context.Context, status, cursor string, limit int) (*repository.Page[domain.BreedingPlan], error) {
	return s.base.repos.Breeding.ListPlans(ctx, s.base.tx.DB(), status, cursor, repository.NormalizeLimit(limit))
}

// AddObservationInput 田间观察入参。
type AddObservationInput struct {
	ObservedAt      string  `json:"observed_at"` // RFC3339，缺省取当前时间
	GerminationRate float64 `json:"germination_rate"`
	Vigor           string  `json:"vigor"`
	Notes           string  `json:"notes"`
}

// AddObservation 为繁育中的计划追加田间观察记录。
func (s *BreedingService) AddObservation(ctx context.Context, actor, planID string, in AddObservationInput) (*domain.FieldObservation, error) {
	if err := domain.ValidateRatio("发芽率", in.GerminationRate); err != nil {
		return nil, err
	}
	now := s.base.now()
	observedAt := now
	if in.ObservedAt != "" {
		t, err := time.Parse(time.RFC3339Nano, in.ObservedAt)
		if err != nil {
			return nil, apperr.Validation("observed_at 必须为 RFC3339 时间")
		}
		observedAt = t.UTC()
	}
	o := &domain.FieldObservation{
		ID:              domain.NewID(domain.PrefixObservation),
		PlanID:          planID,
		ObservedAt:      observedAt,
		GerminationRate: in.GerminationRate,
		Vigor:           in.Vigor,
		Notes:           in.Notes,
		CreatedAt:       now,
	}
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		p, err := s.base.repos.Breeding.GetPlan(ctx, tx, planID)
		if err != nil {
			return err
		}
		if p.Status != domain.PlanActive {
			return apperr.Statef("繁育计划状态 %s 不允许追加田间观察", p.Status)
		}
		if err := s.base.repos.Breeding.InsertObservation(ctx, tx, o); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "breeding.observe", "breeding_plan", planID, o, now)
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

// ListObservations 查询计划的田间观察记录。
func (s *BreedingService) ListObservations(ctx context.Context, planID string) ([]domain.FieldObservation, error) {
	if _, err := s.base.repos.Breeding.GetPlan(ctx, s.base.tx.DB(), planID); err != nil {
		return nil, err
	}
	return s.base.repos.Breeding.ListObservations(ctx, s.base.tx.DB(), planID)
}

// ClosePlan 关闭繁育计划（终态动作，乐观锁）。
func (s *BreedingService) ClosePlan(ctx context.Context, actor, planID string, expectedVersion int64) (*domain.BreedingPlan, error) {
	now := s.base.now()
	var p *domain.BreedingPlan
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		p, err = s.base.repos.Breeding.GetPlan(ctx, tx, planID)
		if err != nil {
			return err
		}
		if p.Version != expectedVersion {
			return apperr.OptimisticLock("繁育计划", planID)
		}
		if err := domain.MustTransition("plan", string(p.Status), string(domain.PlanClosed)); err != nil {
			return err
		}
		if err := s.base.repos.Breeding.UpdatePlanStatus(ctx, tx, p.ID, domain.PlanClosed, p.Version, now); err != nil {
			return err
		}
		p.Status = domain.PlanClosed
		p.Version++
		if err := s.base.repos.Snapshots.Add(ctx, tx, "breeding_plan", p.ID, "CLOSED", p, now); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "breeding.close", "breeding_plan", p.ID, p, now)
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}
