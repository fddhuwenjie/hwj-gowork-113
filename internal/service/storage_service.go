package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"germplasm/internal/apperr"
	"germplasm/internal/domain"
	"germplasm/internal/repository"
)

// StorageService 负责批次建立、样本分装与库位分配。
type StorageService struct {
	base baseService
}

// CreateBatchInput 批次建立入参。
type CreateBatchInput struct {
	AccessionID string `json:"accession_id"`
	BatchNo     string `json:"batch_no"`
	Unit        string `json:"unit"`
	QtyTotal    int64  `json:"qty_total"`
}

// CreateOriginalBatch 在 accession 下建立原始批次，初始数量全部可用。
func (s *StorageService) CreateOriginalBatch(ctx context.Context, actor string, in CreateBatchInput) (*domain.Batch, error) {
	if err := requireNonEmpty("accession", in.AccessionID); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("批次编号", in.BatchNo); err != nil {
		return nil, err
	}
	if err := requirePositive("批次数量", in.QtyTotal); err != nil {
		return nil, err
	}
	unit := in.Unit
	if unit == "" {
		unit = "粒"
	}
	now := s.base.now()
	b := &domain.Batch{
		ID:           domain.NewID(domain.PrefixBatch),
		AccessionID:  in.AccessionID,
		BatchNo:      in.BatchNo,
		Kind:         domain.BatchOriginal,
		Unit:         unit,
		QtyTotal:     in.QtyTotal,
		QtyAvailable: in.QtyTotal,
		Status:       domain.BatchActive,
		Version:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		acc, err := s.base.repos.Resources.GetAccession(ctx, tx, in.AccessionID)
		if err != nil {
			return err
		}
		if err := s.base.repos.Batches.Insert(ctx, tx, b); err != nil {
			return err
		}
		if acc.Status == domain.AccessionRegistered {
			if err := s.base.repos.Resources.UpdateAccessionStatus(ctx, tx, acc.ID, domain.AccessionInStock, acc.Version, now); err != nil {
				return err
			}
		}
		return s.base.audit.Log(ctx, tx, actor, "batch.create", "batch", b.ID, b, now)
	})
	if err != nil {
		return nil, err
	}
	return b, nil
}

// GetBatch 查询批次详情。
func (s *StorageService) GetBatch(ctx context.Context, id string) (*domain.Batch, error) {
	return s.base.repos.Batches.Get(ctx, s.base.tx.DB(), id)
}

// ListBatches 分页查询批次。
func (s *StorageService) ListBatches(ctx context.Context, accessionID, status, cursor string, limit int) (*repository.Page[domain.Batch], error) {
	return s.base.repos.Batches.List(ctx, s.base.tx.DB(), accessionID, status, cursor, repository.NormalizeLimit(limit))
}

// SplitSamplesInput 样本分装入参。
type SplitSamplesInput struct {
	BatchID    string  `json:"batch_id"`
	Quantities []int64 `json:"quantities"` // 每个样本的数量，总和不得超过批次未分装可用量
}

// SplitSamples 将批次可用数量分装为样本。数量必须守恒：
// 分装总和 = 批次可用量 - 已在库样本量；样本不产生也不消灭数量。
func (s *StorageService) SplitSamples(ctx context.Context, actor string, in SplitSamplesInput) ([]domain.Sample, error) {
	if len(in.Quantities) == 0 {
		return nil, apperr.Validation("分装数量列表不能为空")
	}
	var total int64
	for _, q := range in.Quantities {
		if err := requirePositive("样本数量", q); err != nil {
			return nil, err
		}
		total += q
	}
	now := s.base.now()
	var created []domain.Sample
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		b, err := s.base.repos.Batches.Get(ctx, tx, in.BatchID)
		if err != nil {
			return err
		}
		if b.Status != domain.BatchActive {
			return apperr.Statef("批次 %s 状态 %s 不允许分装", b.BatchNo, b.Status)
		}
		sums, err := s.base.repos.Samples.SumByBatchAndStatus(ctx, tx, b.ID)
		if err != nil {
			return err
		}
		unpackaged := b.QtyAvailable - sums[domain.SampleInStock]
		if total > unpackaged {
			return apperr.Quantityf("分装总量 %d 超过批次未分装可用量 %d", total, unpackaged)
		}
		existing, err := s.base.repos.Samples.ListByBatch(ctx, tx, b.ID, "")
		if err != nil {
			return err
		}
		for i, q := range in.Quantities {
			smp := domain.Sample{
				ID:        domain.NewID(domain.PrefixSample),
				BatchID:   b.ID,
				SampleNo:  fmt.Sprintf("%s-S%04d", b.BatchNo, len(existing)+i+1),
				Qty:       q,
				Status:    domain.SampleInStock,
				Version:   1,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := s.base.repos.Samples.Insert(ctx, tx, &smp); err != nil {
				return err
			}
			created = append(created, smp)
		}
		return s.base.audit.Log(ctx, tx, actor, "sample.split", "batch", b.ID, map[string]any{
			"quantities": in.Quantities, "count": len(created),
		}, now)
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// GetSample 查询样本详情。
func (s *StorageService) GetSample(ctx context.Context, id string) (*domain.Sample, error) {
	return s.base.repos.Samples.Get(ctx, s.base.tx.DB(), id)
}

// ListSamples 分页查询样本。
func (s *StorageService) ListSamples(ctx context.Context, batchID, status, cursor string, limit int) (*repository.Page[domain.Sample], error) {
	return s.base.repos.Samples.List(ctx, s.base.tx.DB(), batchID, status, cursor, repository.NormalizeLimit(limit))
}

// CreateLocationInput 库位创建入参。
type CreateLocationInput struct {
	Code     string `json:"code"`
	Chamber  string `json:"chamber"`
	Rack     string `json:"rack"`
	Box      string `json:"box"`
	Slot     string `json:"slot"`
	Capacity int64  `json:"capacity"`
}

// CreateLocation 创建低温库位。
func (s *StorageService) CreateLocation(ctx context.Context, actor string, in CreateLocationInput) (*domain.Location, error) {
	if err := requireNonEmpty("库位编码", in.Code); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("冷库编号", in.Chamber); err != nil {
		return nil, err
	}
	if err := requirePositive("库位容量", in.Capacity); err != nil {
		return nil, err
	}
	now := s.base.now()
	l := &domain.Location{
		ID:        domain.NewID(domain.PrefixLocation),
		Code:      in.Code,
		Chamber:   in.Chamber,
		Rack:      in.Rack,
		Box:       in.Box,
		Slot:      in.Slot,
		Capacity:  in.Capacity,
		Status:    domain.LocationIdle,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		if err := s.base.repos.Locations.Insert(ctx, tx, l); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "location.create", "location", l.ID, l, now)
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}

// ListLocations 分页查询库位。
func (s *StorageService) ListLocations(ctx context.Context, chamber, cursor string, limit int) (*repository.Page[domain.Location], error) {
	return s.base.repos.Locations.List(ctx, s.base.tx.DB(), chamber, cursor, repository.NormalizeLimit(limit))
}

// AssignLocation 将样本分配到库位：库位占用 +1，样本记录库位。
func (s *StorageService) AssignLocation(ctx context.Context, actor, sampleID, locationID string, expectedVersion int64) (*domain.Sample, error) {
	now := s.base.now()
	var smp *domain.Sample
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		smp, err = s.base.repos.Samples.Get(ctx, tx, sampleID)
		if err != nil {
			return err
		}
		if smp.Version != expectedVersion {
			return apperr.OptimisticLock("样本", sampleID)
		}
		if smp.Status != domain.SampleInStock {
			return apperr.Statef("样本状态 %s 不允许分配库位", smp.Status)
		}
		if smp.LocationID != "" {
			return apperr.Statef("样本已分配库位 %s", smp.LocationID)
		}
		loc, err := s.base.repos.Locations.Get(ctx, tx, locationID)
		if err != nil {
			var ae *apperr.Error
			if errors.As(err, &ae) && ae.Code == apperr.CodeNotFound {
				return apperr.NotFound("库位", locationID)
			}
			return err
		}
		if err := s.base.repos.Locations.Occupy(ctx, tx, loc.ID, loc.Version, now); err != nil {
			return err
		}
		smp.LocationID = loc.ID
		if err := s.base.repos.Samples.Update(ctx, tx, smp, expectedVersion, now); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "sample.assign_location", "sample", smp.ID, map[string]any{
			"location_id": loc.ID, "location_code": loc.Code,
		}, now)
	})
	if err != nil {
		return nil, err
	}
	return smp, nil
}
