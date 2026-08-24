package repository

import (
	"context"

	"germplasm/internal/apperr"
	"germplasm/internal/clock"
	"germplasm/internal/domain"
	"germplasm/internal/store"
)

// LineageRepo 负责资源谱系边持久化与异常检测。
type LineageRepo struct{}

// NewLineageRepo 创建仓储。
func NewLineageRepo() *LineageRepo { return &LineageRepo{} }

// InsertEdge 插入谱系边。
func (r *LineageRepo) InsertEdge(ctx context.Context, q store.Queryer, e *domain.LineageEdge) error {
	_, err := q.ExecContext(ctx, `INSERT INTO lineage_edges (id, resource_id, parent_batch_id, child_batch_id, relation, created_at)
		VALUES (?,?,?,?,?,?)`, e.ID, e.ResourceID, e.ParentBatchID, e.ChildBatchID, e.Relation, clock.Format(e.CreatedAt))
	if isUniqueViolation(err) {
		return apperr.Conflict("谱系边已存在")
	}
	return err
}

// ListParents 查询子批的全部母批边。
func (r *LineageRepo) ListParents(ctx context.Context, q store.Queryer, childBatchID string) ([]domain.LineageEdge, error) {
	return r.query(ctx, q, `SELECT id, resource_id, parent_batch_id, child_batch_id, relation, created_at
		FROM lineage_edges WHERE child_batch_id = ? ORDER BY created_at, id`, childBatchID)
}

// ListChildren 查询母批的全部子批边。
func (r *LineageRepo) ListChildren(ctx context.Context, q store.Queryer, parentBatchID string) ([]domain.LineageEdge, error) {
	return r.query(ctx, q, `SELECT id, resource_id, parent_batch_id, child_batch_id, relation, created_at
		FROM lineage_edges WHERE parent_batch_id = ? ORDER BY created_at, id`, parentBatchID)
}

// ListAll 查询全部谱系边（异常巡检用）。
func (r *LineageRepo) ListAll(ctx context.Context, q store.Queryer) ([]domain.LineageEdge, error) {
	return r.query(ctx, q, `SELECT id, resource_id, parent_batch_id, child_batch_id, relation, created_at
		FROM lineage_edges ORDER BY created_at, id`)
}

func (r *LineageRepo) query(ctx context.Context, q store.Queryer, sql string, args ...any) ([]domain.LineageEdge, error) {
	rows, err := q.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.LineageEdge
	for rows.Next() {
		var e domain.LineageEdge
		var createdAt string
		if err := rows.Scan(&e.ID, &e.ResourceID, &e.ParentBatchID, &e.ChildBatchID, &e.Relation, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt = clock.MustParse(createdAt)
		items = append(items, e)
	}
	return items, rows.Err()
}

// Anomaly 描述一条谱系异常。
type Anomaly struct {
	Type    string `json:"type"` // CYCLE / SELF_LOOP / ORPHAN
	BatchID string `json:"batch_id"`
	Message string `json:"message"`
}

// DetectAnomalies 检测谱系异常：自环、成环、回存批次缺少母批。
// orphanBatches 为无谱系边但类型为 RESTOCK/REGENERATION 的批次。
func (r *LineageRepo) DetectAnomalies(edges []domain.LineageEdge, orphanBatches []domain.Batch) []Anomaly {
	var out []Anomaly
	adj := map[string][]string{}
	for _, e := range edges {
		if e.ParentBatchID == e.ChildBatchID {
			out = append(out, Anomaly{Type: "SELF_LOOP", BatchID: e.ChildBatchID, Message: "批次谱系存在自环"})
			continue
		}
		adj[e.ParentBatchID] = append(adj[e.ParentBatchID], e.ChildBatchID)
	}
	// 三色标记法检测有向环。
	const (
		white = 0 // 未访问
		gray  = 1 // 访问中
		black = 2 // 已完成
	)
	color := map[string]int{}
	seen := map[string]bool{}
	var visit func(n string) bool
	visit = func(n string) bool {
		if color[n] == gray {
			return true
		}
		if color[n] == black {
			return false
		}
		color[n] = gray
		for _, m := range adj[n] {
			if visit(m) {
				if !seen[n] {
					out = append(out, Anomaly{Type: "CYCLE", BatchID: n, Message: "批次谱系存在环"})
					seen[n] = true
				}
			}
		}
		color[n] = black
		return false
	}
	for n := range adj {
		visit(n)
	}
	for _, b := range orphanBatches {
		out = append(out, Anomaly{Type: "ORPHAN", BatchID: b.ID, Message: "回存/繁育批次缺少母批谱系"})
	}
	return out
}
