package service

import (
	"context"

	"germplasm/internal/domain"
	"germplasm/internal/repository"
)

// LineageService 负责资源谱系查询与异常检测。
type LineageService struct {
	base baseService
}

// LineageView 为某批次的谱系视图：母批与子批。
type LineageView struct {
	BatchID  string               `json:"batch_id"`
	Parents  []domain.LineageEdge `json:"parents"`
	Children []domain.LineageEdge `json:"children"`
}

// GetLineage 查询批次的直接谱系关联。
func (s *LineageService) GetLineage(ctx context.Context, batchID string) (*LineageView, error) {
	if _, err := s.base.repos.Batches.Get(ctx, s.base.tx.DB(), batchID); err != nil {
		return nil, err
	}
	parents, err := s.base.repos.Lineage.ListParents(ctx, s.base.tx.DB(), batchID)
	if err != nil {
		return nil, err
	}
	children, err := s.base.repos.Lineage.ListChildren(ctx, s.base.tx.DB(), batchID)
	if err != nil {
		return nil, err
	}
	return &LineageView{BatchID: batchID, Parents: parents, Children: children}, nil
}

// Anomalies 检测全库谱系异常：自环、有向环、缺少母批的回存/繁育批次。
func (s *LineageService) Anomalies(ctx context.Context) ([]repository.Anomaly, error) {
	edges, err := s.base.repos.Lineage.ListAll(ctx, s.base.tx.DB())
	if err != nil {
		return nil, err
	}
	batches, err := s.base.repos.Batches.ListAll(ctx, s.base.tx.DB())
	if err != nil {
		return nil, err
	}
	hasParent := map[string]bool{}
	for _, e := range edges {
		hasParent[e.ChildBatchID] = true
	}
	var orphans []domain.Batch
	for _, b := range batches {
		if (b.Kind == domain.BatchRestock || b.Kind == domain.BatchRegeneration) && !hasParent[b.ID] {
			orphans = append(orphans, b)
		}
	}
	return s.base.repos.Lineage.DetectAnomalies(edges, orphans), nil
}
