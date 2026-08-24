package service

import (
	"context"
	"database/sql"

	"germplasm/internal/apperr"
	"germplasm/internal/domain"
	"germplasm/internal/repository"
)

// RuleService 负责保存规则版本管理。
type RuleService struct {
	base baseService
}

// CreateRuleInput 规则版本创建入参。
type CreateRuleInput struct {
	Code              string  `json:"code"`
	MinTemp           float64 `json:"min_temp"`
	MaxTemp           float64 `json:"max_temp"`
	MinHumidity       float64 `json:"min_humidity"`
	MaxHumidity       float64 `json:"max_humidity"`
	WindowBeforeHours int     `json:"window_before_hours"`
	WindowAfterHours  int     `json:"window_after_hours"`
	MinCoverage       float64 `json:"min_coverage"`
	MinPurity         float64 `json:"min_purity"`
}

// CreateRuleVersion 创建规则草稿版本，版本号自动递增。
func (s *RuleService) CreateRuleVersion(ctx context.Context, actor string, in CreateRuleInput) (*domain.RuleVersion, error) {
	if err := requireNonEmpty("规则编码", in.Code); err != nil {
		return nil, err
	}
	if in.MinTemp >= in.MaxTemp {
		return nil, apperr.Validation("min_temp 必须小于 max_temp")
	}
	if in.MinHumidity >= in.MaxHumidity {
		return nil, apperr.Validation("min_humidity 必须小于 max_humidity")
	}
	if err := domain.ValidateRatio("min_coverage", in.MinCoverage); err != nil {
		return nil, err
	}
	if err := domain.ValidateRatio("min_purity", in.MinPurity); err != nil {
		return nil, err
	}
	if in.WindowBeforeHours < 0 || in.WindowAfterHours < 0 {
		return nil, apperr.Validation("监控窗口小时数不得为负")
	}
	now := s.base.now()
	var rv *domain.RuleVersion
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		next, err := s.base.repos.Rules.NextVersionNo(ctx, tx, in.Code)
		if err != nil {
			return err
		}
		rv = &domain.RuleVersion{
			ID:                domain.NewID(domain.PrefixRule),
			Code:              in.Code,
			VersionNo:         next,
			MinTemp:           in.MinTemp,
			MaxTemp:           in.MaxTemp,
			MinHumidity:       in.MinHumidity,
			MaxHumidity:       in.MaxHumidity,
			WindowBeforeHours: in.WindowBeforeHours,
			WindowAfterHours:  in.WindowAfterHours,
			MinCoverage:       in.MinCoverage,
			MinPurity:         in.MinPurity,
			Status:            domain.RuleDraft,
			CreatedAt:         now,
		}
		if err := s.base.repos.Rules.Insert(ctx, tx, rv); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "rule.create", "rule_version", rv.ID, rv, now)
	})
	if err != nil {
		return nil, err
	}
	return rv, nil
}

// ActivateRule 启用规则版本：同 code 旧版本退役，新版本生效。
// 启用为终态前置转换：仅 DRAFT 可启用，退役版本再次启用将整体回滚并返回 STATE_CONFLICT，
// 当前生效版本保持不变。返回值始终反映事务提交后的真实持久化状态，不做内存改写。
func (s *RuleService) ActivateRule(ctx context.Context, actor, id string) (*domain.RuleVersion, error) {
	now := s.base.now()
	var rv *domain.RuleVersion
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		if err := s.base.repos.Rules.Activate(ctx, tx, id, now); err != nil {
			return err
		}
		// 在同一事务内回读提交后的真实状态，保证返回值与持久化一致。
		current, err := s.base.repos.Rules.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if current.Status != domain.RuleActive {
			// 理论不可达：转换已校验且 UPDATE 作用于当前 DRAFT 行。
			return apperr.Newf(500, apperr.CodeInternal, "启用规则 %s 后状态异常: %s", id, current.Status)
		}
		rv = current
		return s.base.audit.Log(ctx, tx, actor, "rule.activate", "rule_version", id, rv, now)
	})
	if err != nil {
		return nil, err
	}
	return rv, nil
}

// GetRule 查询规则版本详情。
func (s *RuleService) GetRule(ctx context.Context, id string) (*domain.RuleVersion, error) {
	return s.base.repos.Rules.Get(ctx, s.base.tx.DB(), id)
}

// ListRules 分页查询规则版本。
func (s *RuleService) ListRules(ctx context.Context, code, cursor string, limit int) (*repository.Page[domain.RuleVersion], error) {
	return s.base.repos.Rules.List(ctx, s.base.tx.DB(), code, cursor, repository.NormalizeLimit(limit))
}
