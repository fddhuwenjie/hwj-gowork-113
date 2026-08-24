package service

import (
	"context"
	"database/sql"
	"errors"

	"germplasm/internal/apperr"
	"germplasm/internal/domain"
	"germplasm/internal/repository"
)

// ResourceService 负责资源登记与 accession 管理。
type ResourceService struct {
	base baseService
}

// CreateResourceInput 资源登记入参。
type CreateResourceInput struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Species  string `json:"species"`
	Category string `json:"category"`
	Remark   string `json:"remark"`
}

// CreateResource 登记新种质资源。
func (s *ResourceService) CreateResource(ctx context.Context, actor string, in CreateResourceInput) (*domain.Resource, error) {
	if err := requireNonEmpty("资源编码", in.Code); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("资源名称", in.Name); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("物种", in.Species); err != nil {
		return nil, err
	}
	now := s.base.now()
	res := &domain.Resource{
		ID:        domain.NewID(domain.PrefixResource),
		Code:      in.Code,
		Name:      in.Name,
		Species:   in.Species,
		Category:  in.Category,
		Status:    domain.ResourceActive,
		Remark:    in.Remark,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		if err := s.base.repos.Resources.InsertResource(ctx, tx, res); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "resource.create", "resource", res.ID, res, now)
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// GetResource 查询资源详情。
func (s *ResourceService) GetResource(ctx context.Context, id string) (*domain.Resource, error) {
	res, err := s.base.repos.Resources.GetResource(ctx, s.base.tx.DB(), id)
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) && ae.Code == apperr.CodeNotFound {
			return nil, apperr.NotFound("资源", id)
		}
		return nil, err
	}
	return res, nil
}

// ListResources 分页查询资源。
func (s *ResourceService) ListResources(ctx context.Context, cursor string, limit int) (*repository.Page[domain.Resource], error) {
	return s.base.repos.Resources.ListResources(ctx, s.base.tx.DB(), cursor, repository.NormalizeLimit(limit))
}

// ArchiveResource 归档资源（乐观锁）。
func (s *ResourceService) ArchiveResource(ctx context.Context, actor, id string, expectedVersion int64) (*domain.Resource, error) {
	var res *domain.Resource
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		res, err = s.base.repos.Resources.GetResourceForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if res.Version != expectedVersion {
			return apperr.OptimisticLock("资源", id)
		}
		res.Status = domain.ResourceArchived
		if err := s.base.repos.Resources.UpdateResource(ctx, tx, res, expectedVersion); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "resource.archive", "resource", id, res, s.base.now())
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// CreateAccessionInput accession 登记入参。
type CreateAccessionInput struct {
	ResourceID  string `json:"resource_id"`
	AccessionNo string `json:"accession_no"`
	Origin      string `json:"origin"`
	Donor       string `json:"donor"`
	CollectedAt string `json:"collected_at"`
}

// CreateAccession 在资源下登记 accession。
func (s *ResourceService) CreateAccession(ctx context.Context, actor string, in CreateAccessionInput) (*domain.Accession, error) {
	if err := requireNonEmpty("资源ID", in.ResourceID); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("种质编号", in.AccessionNo); err != nil {
		return nil, err
	}
	now := s.base.now()
	a := &domain.Accession{
		ID:          domain.NewID(domain.PrefixAccession),
		ResourceID:  in.ResourceID,
		AccessionNo: in.AccessionNo,
		Origin:      in.Origin,
		Donor:       in.Donor,
		CollectedAt: in.CollectedAt,
		Status:      domain.AccessionRegistered,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		res, err := s.base.repos.Resources.GetResource(ctx, tx, in.ResourceID)
		if err != nil {
			return err
		}
		if res.Status != domain.ResourceActive {
			return apperr.Statef("资源 %s 已归档，不能登记 accession", in.ResourceID)
		}
		if err := s.base.repos.Resources.InsertAccession(ctx, tx, a); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "accession.create", "accession", a.ID, a, now)
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

// GetAccession 查询 accession 详情。
func (s *ResourceService) GetAccession(ctx context.Context, id string) (*domain.Accession, error) {
	return s.base.repos.Resources.GetAccession(ctx, s.base.tx.DB(), id)
}

// ListAccessions 分页查询 accession。
func (s *ResourceService) ListAccessions(ctx context.Context, resourceID, cursor string, limit int) (*repository.Page[domain.Accession], error) {
	return s.base.repos.Resources.ListAccessions(ctx, s.base.tx.DB(), resourceID, cursor, repository.NormalizeLimit(limit))
}
