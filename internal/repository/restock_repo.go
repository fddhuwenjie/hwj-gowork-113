package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"germplasm/internal/apperr"
	"germplasm/internal/clock"
	"germplasm/internal/domain"
	"germplasm/internal/store"
)

// RestockRepo 负责回存验收单持久化。
type RestockRepo struct{}

// NewRestockRepo 创建仓储。
func NewRestockRepo() *RestockRepo { return &RestockRepo{} }

const restockCols = `id, request_no, plan_id, qty, status, new_batch_id, reject_reason, idempotency_key, version, created_at, updated_at`

// Insert 插入回存验收单。
func (r *RestockRepo) Insert(ctx context.Context, q store.Queryer, rb *domain.RestockBatch) error {
	var key *string
	if rb.IdempotencyKey != "" {
		key = &rb.IdempotencyKey
	}
	_, err := q.ExecContext(ctx, `INSERT INTO restock_batches (`+restockCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		rb.ID, rb.RequestNo, rb.PlanID, rb.Qty, string(rb.Status), rb.NewBatchID, rb.RejectReason, key,
		rb.Version, clock.Format(rb.CreatedAt), clock.Format(rb.UpdatedAt))
	if isUniqueViolation(err) {
		return apperr.Conflict("回存单号或幂等键已存在")
	}
	return err
}

// Get 按 ID 查询回存验收单。
func (r *RestockRepo) Get(ctx context.Context, q store.Queryer, id string) (*domain.RestockBatch, error) {
	row := q.QueryRowContext(ctx, `SELECT `+restockCols+` FROM restock_batches WHERE id = ?`, id)
	return scanRestock(row)
}

// GetByIdempotencyKey 按幂等键查询，不存在时返回 nil, nil。
func (r *RestockRepo) GetByIdempotencyKey(ctx context.Context, q store.Queryer, key string) (*domain.RestockBatch, error) {
	row := q.QueryRowContext(ctx, `SELECT `+restockCols+` FROM restock_batches WHERE idempotency_key = ?`, key)
	rb, err := scanRestock(row)
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) && ae.Code == apperr.CodeNotFound {
			return nil, nil
		}
		return nil, err
	}
	return rb, nil
}

func scanRestock(row *sql.Row) (*domain.RestockBatch, error) {
	var rb domain.RestockBatch
	var status, createdAt, updatedAt string
	var key *string
	err := row.Scan(&rb.ID, &rb.RequestNo, &rb.PlanID, &rb.Qty, &status, &rb.NewBatchID, &rb.RejectReason, &key,
		&rb.Version, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("回存验收单", "")
	}
	if err != nil {
		return nil, err
	}
	rb.Status = domain.RestockStatus(status)
	if key != nil {
		rb.IdempotencyKey = *key
	}
	rb.CreatedAt = clock.MustParse(createdAt)
	rb.UpdatedAt = clock.MustParse(updatedAt)
	return &rb, nil
}

// UpdateStatus 乐观锁更新回存验收单状态。
func (r *RestockRepo) UpdateStatus(ctx context.Context, q store.Queryer, id string, status domain.RestockStatus, newBatchID, rejectReason string, expectedVersion int64, now time.Time) error {
	if status == domain.RestockAccepted {
		newBatchID = ""
	}
	r2, err := q.ExecContext(ctx, `UPDATE restock_batches SET status=?, new_batch_id=?, reject_reason=?, version=version+1, updated_at=?
		WHERE id=? AND version=?`,
		string(status), newBatchID, rejectReason, clock.Format(now), id, expectedVersion)
	if err != nil {
		return err
	}
	return ensureOneRow(r2, "回存验收单", id)
}

// List 稳定分页查询回存验收单。
func (r *RestockRepo) List(ctx context.Context, q store.Queryer, status, cursor string, limit int) (*Page[domain.RestockBatch], error) {
	c, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	cond, cursorArgs := cursorCondition(c)
	var args []any
	where := " WHERE 1=1"
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	rows, err := q.QueryContext(ctx, `SELECT `+restockCols+` FROM restock_batches`+where+cond+
		` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.RestockBatch
	for rows.Next() {
		rb, err := scanRestockRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *rb)
	}
	return paginate(items, limit, func(rb domain.RestockBatch) (time.Time, string) { return rb.CreatedAt, rb.ID })
}

func scanRestockRows(rows *sql.Rows) (*domain.RestockBatch, error) {
	var rb domain.RestockBatch
	var status, createdAt, updatedAt string
	var key *string
	if err := rows.Scan(&rb.ID, &rb.RequestNo, &rb.PlanID, &rb.Qty, &status, &rb.NewBatchID, &rb.RejectReason, &key,
		&rb.Version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	rb.Status = domain.RestockStatus(status)
	if key != nil {
		rb.IdempotencyKey = *key
	}
	rb.CreatedAt = clock.MustParse(createdAt)
	rb.UpdatedAt = clock.MustParse(updatedAt)
	return &rb, nil
}

// ListPendingOlderThan 查询创建时间早于 threshold 的待验收回存单（超期巡检用）。
func (r *RestockRepo) ListPendingOlderThan(ctx context.Context, q store.Queryer, threshold time.Time) ([]domain.RestockBatch, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+restockCols+` FROM restock_batches
		WHERE status='PENDING' AND created_at < ? ORDER BY created_at, id`, clock.Format(threshold))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.RestockBatch
	for rows.Next() {
		rb, err := scanRestockRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *rb)
	}
	return items, rows.Err()
}
