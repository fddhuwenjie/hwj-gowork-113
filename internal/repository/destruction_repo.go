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

// DestructionRepo 负责销毁审批单持久化。
type DestructionRepo struct{}

// NewDestructionRepo 创建仓储。
func NewDestructionRepo() *DestructionRepo { return &DestructionRepo{} }

const destructionCols = `id, batch_id, qty, reason, status, approver, version, created_at, updated_at`

// Insert 插入销毁审批单。
func (r *DestructionRepo) Insert(ctx context.Context, q store.Queryer, d *domain.DestructionApproval) error {
	_, err := q.ExecContext(ctx, `INSERT INTO destruction_approvals (`+destructionCols+`) VALUES (?,?,?,?,?,?,?,?,?)`,
		d.ID, d.BatchID, d.Qty, d.Reason, string(d.Status), d.Approver, d.Version,
		clock.Format(d.CreatedAt), clock.Format(d.UpdatedAt))
	return err
}

// Get 按 ID 查询销毁审批单。
func (r *DestructionRepo) Get(ctx context.Context, q store.Queryer, id string) (*domain.DestructionApproval, error) {
	row := q.QueryRowContext(ctx, `SELECT `+destructionCols+` FROM destruction_approvals WHERE id = ?`, id)
	var d domain.DestructionApproval
	var status, createdAt, updatedAt string
	err := row.Scan(&d.ID, &d.BatchID, &d.Qty, &d.Reason, &status, &d.Approver, &d.Version, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("销毁审批单", id)
	}
	if err != nil {
		return nil, err
	}
	d.Status = domain.DestructionStatus(status)
	d.CreatedAt = clock.MustParse(createdAt)
	d.UpdatedAt = clock.MustParse(updatedAt)
	return &d, nil
}

// UpdateStatus 乐观锁更新销毁审批单状态。
func (r *DestructionRepo) UpdateStatus(ctx context.Context, q store.Queryer, id string, status domain.DestructionStatus, approver string, expectedVersion int64, now time.Time) error {
	r2, err := q.ExecContext(ctx, `UPDATE destruction_approvals SET status=?, approver=?, version=version+1, updated_at=?
		WHERE id=? AND version=?`, string(status), approver, clock.Format(now), id, expectedVersion)
	if err != nil {
		return err
	}
	return ensureOneRow(r2, "销毁审批单", id)
}

// List 稳定分页查询销毁审批单。
func (r *DestructionRepo) List(ctx context.Context, q store.Queryer, batchID, status, cursor string, limit int) (*Page[domain.DestructionApproval], error) {
	c, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	cond, cursorArgs := cursorCondition(c)
	var args []any
	where := " WHERE 1=1"
	if batchID != "" {
		where += " AND batch_id = ?"
		args = append(args, batchID)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	rows, err := q.QueryContext(ctx, `SELECT `+destructionCols+` FROM destruction_approvals`+where+cond+
		` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.DestructionApproval
	for rows.Next() {
		var d domain.DestructionApproval
		var st, createdAt, updatedAt string
		if err := rows.Scan(&d.ID, &d.BatchID, &d.Qty, &d.Reason, &st, &d.Approver, &d.Version, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		d.Status = domain.DestructionStatus(st)
		d.CreatedAt = clock.MustParse(createdAt)
		d.UpdatedAt = clock.MustParse(updatedAt)
		items = append(items, d)
	}
	return paginate(items, limit, func(d domain.DestructionApproval) (time.Time, string) { return d.CreatedAt, d.ID })
}
