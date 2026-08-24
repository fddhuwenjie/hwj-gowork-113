package service

import (
	"context"
	"database/sql"
	"time"

	"germplasm/internal/apperr"
	"germplasm/internal/domain"
	"germplasm/internal/repository"
)

// PurityService 负责纯度检测登记、质量判定与封存。
type PurityService struct {
	base baseService
}

// CreateTestInput 纯度检测入参。
type CreateTestInput struct {
	PlanID         string  `json:"plan_id"`
	SampleQty      int64   `json:"sample_qty"`
	CoverageRatio  float64 `json:"coverage_ratio"`
	PurityRate     float64 `json:"purity_rate"`
	TestedAt       string  `json:"tested_at"` // RFC3339，缺省取当前时间
	IdempotencyKey string  `json:"idempotency_key"`
}

// CreateTest 登记一次纯度检测（未判定）。重复提交凭幂等键返回首条记录。
// 若计划已有封存结论，而本次检测时间早于封存时间，属于迟到检测，
// 仅作只读记录，永远不得参与质量判定。
func (s *PurityService) CreateTest(ctx context.Context, actor string, in CreateTestInput) (*domain.PurityTest, error) {
	if err := requireNonEmpty("繁育计划", in.PlanID); err != nil {
		return nil, err
	}
	if err := requirePositive("抽样数量", in.SampleQty); err != nil {
		return nil, err
	}
	if err := domain.ValidateRatio("检测覆盖率", in.CoverageRatio); err != nil {
		return nil, err
	}
	if err := domain.ValidateRatio("纯度合格率", in.PurityRate); err != nil {
		return nil, err
	}
	now := s.base.now()
	testedAt := now
	if in.TestedAt != "" {
		t, err := time.Parse(time.RFC3339Nano, in.TestedAt)
		if err != nil {
			return nil, apperr.Validation("tested_at 必须为 RFC3339 时间")
		}
		testedAt = t.UTC()
	}
	reqHash := repository.HashRequest(in)
	t := &domain.PurityTest{
		ID:             domain.NewID(domain.PrefixPurityTest),
		PlanID:         in.PlanID,
		SampleQty:      in.SampleQty,
		CoverageRatio:  in.CoverageRatio,
		PurityRate:     in.PurityRate,
		Verdict:        domain.VerdictPending,
		TestedAt:       testedAt,
		IdempotencyKey: in.IdempotencyKey,
		Version:        1,
		CreatedAt:      now,
	}
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		if id, found, err := s.base.idemLookup(ctx, tx, in.IdempotencyKey, "purity.create", reqHash); err != nil {
			return err
		} else if found {
			t.ID = id
			return errReplay
		}
		p, err := s.base.repos.Breeding.GetPlan(ctx, tx, in.PlanID)
		if err != nil {
			return err
		}
		if p.Status == domain.PlanClosed {
			return apperr.Statef("繁育计划已关闭，不能登记检测")
		}
		if err := s.base.repos.Purity.Insert(ctx, tx, t); err != nil {
			return err
		}
		if err := s.base.idemSave(ctx, tx, in.IdempotencyKey, "purity.create", reqHash, t.ID); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "purity.create", "purity_test", t.ID, t, now)
	})
	if err != nil {
		if err == errReplay {
			return s.base.repos.Purity.Get(ctx, s.base.tx.DB(), t.ID)
		}
		return nil, err
	}
	return t, nil
}

// SealTest 封存质量判定：依据计划母批绑定的保存规则计算结论并封存。
// 迟到检测（tested_at 早于既有封存时刻）不得封存、不得覆盖当前结论。
// 封存与快照、审计在同一事务内完成，失败整体回滚。
func (s *PurityService) SealTest(ctx context.Context, actor, testID string, expectedVersion int64) (*domain.PurityTest, error) {
	now := s.base.now()
	var t *domain.PurityTest
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		t, err = s.base.repos.Purity.Get(ctx, tx, testID)
		if err != nil {
			return err
		}
		if t.Version != expectedVersion {
			return apperr.OptimisticLock("纯度检测", testID)
		}
		if t.Sealed {
			return apperr.Statef("检测 %s 已封存", testID)
		}
		sealed, err := s.base.repos.Purity.LatestSealed(ctx, tx, t.PlanID)
		if err != nil {
			return err
		}
		if sealed != nil {
			return apperr.Statef("计划 %s 已存在封存结论，禁止再次封存", t.PlanID)
		}
		plan, err := s.base.repos.Breeding.GetPlan(ctx, tx, t.PlanID)
		if err != nil {
			return err
		}
		o, err := s.base.repos.Outbound.Get(ctx, tx, plan.OutboundRequestID)
		if err != nil {
			return err
		}
		rule, err := s.base.repos.Rules.Get(ctx, tx, o.RuleVersionID)
		if err != nil {
			return err
		}
		verdict := domain.JudgeVerdict(rule, t.CoverageRatio, t.PurityRate)
		if err := s.base.repos.Purity.Seal(ctx, tx, t.ID, verdict, expectedVersion, now); err != nil {
			return err
		}
		t.Verdict = verdict
		t.Sealed = true
		t.Version++
		sealedAt := now
		t.SealedAt = &sealedAt
		if err := s.base.repos.Snapshots.Add(ctx, tx, "purity_test", t.ID, "SEALED", map[string]any{
			"test": t, "rule": rule,
		}, now); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "purity.seal", "purity_test", t.ID, t, now)
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetTest 查询检测详情。
func (s *PurityService) GetTest(ctx context.Context, id string) (*domain.PurityTest, error) {
	return s.base.repos.Purity.Get(ctx, s.base.tx.DB(), id)
}

// ListTests 分页查询检测，按创建时间稳定升序游标翻页。
// limit 规整交给仓储：仓储内部用 limit+1 探测下一页是否存在，
// 故此处不得再叠加 limit+1，否则单页会多返回一条。
func (s *PurityService) ListTests(ctx context.Context, planID, cursor string, limit int) (*repository.Page[domain.PurityTest], error) {
	return s.base.repos.Purity.List(ctx, s.base.tx.DB(), planID, cursor, repository.NormalizeLimit(limit))
}

// IsLateTest 判断检测是否属于迟到检测（早于既有封存时刻）。
func (s *PurityService) IsLateTest(ctx context.Context, t *domain.PurityTest) (bool, error) {
	sealed, err := s.base.repos.Purity.LatestSealed(ctx, s.base.tx.DB(), t.PlanID)
	if err != nil || sealed == nil || sealed.SealedAt == nil {
		return false, err
	}
	return t.TestedAt.Before(*sealed.SealedAt) && !t.Sealed, nil
}
