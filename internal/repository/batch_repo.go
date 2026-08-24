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

// BatchRepo 负责批次持久化与数量守恒校验。
type BatchRepo struct{}

// NewBatchRepo 创建仓储。
func NewBatchRepo() *BatchRepo { return &BatchRepo{} }

const batchCols = `id, accession_id, batch_no, kind, mother_batch_id, unit, qty_total, qty_available,
	qty_frozen, qty_outbound, qty_destroyed, status, version, created_at, updated_at, closed_at`

// Insert 插入批次。
func (r *BatchRepo) Insert(ctx context.Context, q store.Queryer, b *domain.Batch) error {
	var closedAt *string
	if b.ClosedAt != nil {
		s := clock.Format(*b.ClosedAt)
		closedAt = &s
	}
	_, err := q.ExecContext(ctx, `INSERT INTO batches (`+batchCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		b.ID, b.AccessionID, b.BatchNo, string(b.Kind), b.MotherBatchID, b.Unit,
		b.QtyTotal, b.QtyAvailable, b.QtyFrozen, b.QtyOutbound, b.QtyDestroyed,
		string(b.Status), b.Version, clock.Format(b.CreatedAt), clock.Format(b.UpdatedAt), closedAt)
	if isUniqueViolation(err) {
		return apperr.Conflict("批次编号已存在: " + b.BatchNo)
	}
	return err
}

// Get 按 ID 查询批次。
func (r *BatchRepo) Get(ctx context.Context, q store.Queryer, id string) (*domain.Batch, error) {
	row := q.QueryRowContext(ctx, `SELECT `+batchCols+` FROM batches WHERE id = ?`, id)
	return scanBatch(row)
}

// GetByNo 按批次编号查询。
func (r *BatchRepo) GetByNo(ctx context.Context, q store.Queryer, no string) (*domain.Batch, error) {
	row := q.QueryRowContext(ctx, `SELECT `+batchCols+` FROM batches WHERE batch_no = ?`, no)
	return scanBatch(row)
}

func scanBatch(row *sql.Row) (*domain.Batch, error) {
	var b domain.Batch
	var kind, status, createdAt, updatedAt string
	var closedAt *string
	err := row.Scan(&b.ID, &b.AccessionID, &b.BatchNo, &kind, &b.MotherBatchID, &b.Unit,
		&b.QtyTotal, &b.QtyAvailable, &b.QtyFrozen, &b.QtyOutbound, &b.QtyDestroyed,
		&status, &b.Version, &createdAt, &updatedAt, &closedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("批次", "")
	}
	if err != nil {
		return nil, err
	}
	b.Kind = domain.BatchKind(kind)
	b.Status = domain.BatchStatus(status)
	b.CreatedAt = clock.MustParse(createdAt)
	b.UpdatedAt = clock.MustParse(updatedAt)
	b.ClosedAt = parseNullTime(closedAt)
	return &b, nil
}

// UpdateQuantities 乐观锁更新批次数量与状态，写库前强制校验数量守恒。
func (r *BatchRepo) UpdateQuantities(ctx context.Context, q store.Queryer, b *domain.Batch, expectedVersion int64, now time.Time) error {
	if b.CheckConservation() != 0 {
		return apperr.Quantityf("批次 %s 数量不守恒，偏差 %d", b.BatchNo, b.CheckConservation())
	}
	if b.QtyAvailable < 0 || b.QtyFrozen < 0 || b.QtyOutbound < 0 || b.QtyDestroyed < 0 {
		return apperr.Quantity("批次数量不得为负")
	}
	var closedAt *string
	if b.ClosedAt != nil {
		s := clock.Format(*b.ClosedAt)
		closedAt = &s
	}
	b.UpdatedAt = now
	r2, err := q.ExecContext(ctx, `UPDATE batches SET qty_available=?, qty_frozen=?, qty_outbound=?, qty_destroyed=?,
		status=?, version=version+1, updated_at=?, closed_at=? WHERE id=? AND version=?`,
		b.QtyAvailable, b.QtyFrozen, b.QtyOutbound, b.QtyDestroyed, string(b.Status),
		clock.Format(b.UpdatedAt), closedAt, b.ID, expectedVersion)
	if err != nil {
		return err
	}
	if err := ensureOneRow(r2, "批次", b.ID); err != nil {
		return err
	}
	b.Version = expectedVersion + 1
	return nil
}

// List 稳定分页查询批次，可按 accession 与状态过滤。
func (r *BatchRepo) List(ctx context.Context, q store.Queryer, accessionID, status, cursor string, limit int) (*Page[domain.Batch], error) {
	c, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	cond, cursorArgs := cursorCondition(c)
	var args []any
	where := " WHERE 1=1"
	if accessionID != "" {
		where += " AND accession_id = ?"
		args = append(args, accessionID)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	rows, err := q.QueryContext(ctx, `SELECT `+batchCols+` FROM batches`+where+cond+
		` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Batch
	for rows.Next() {
		b, err := scanBatchRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *b)
	}
	return paginate(items, limit, func(b domain.Batch) (time.Time, string) { return b.CreatedAt, b.ID })
}

func scanBatchRows(rows *sql.Rows) (*domain.Batch, error) {
	var b domain.Batch
	var kind, status, createdAt, updatedAt string
	var closedAt *string
	if err := rows.Scan(&b.ID, &b.AccessionID, &b.BatchNo, &kind, &b.MotherBatchID, &b.Unit,
		&b.QtyTotal, &b.QtyAvailable, &b.QtyFrozen, &b.QtyOutbound, &b.QtyDestroyed,
		&status, &b.Version, &createdAt, &updatedAt, &closedAt); err != nil {
		return nil, err
	}
	b.Kind = domain.BatchKind(kind)
	b.Status = domain.BatchStatus(status)
	b.CreatedAt = clock.MustParse(createdAt)
	b.UpdatedAt = clock.MustParse(updatedAt)
	b.ClosedAt = parseNullTime(closedAt)
	return &b, nil
}

// ListAll 查询全部批次（用于库存差异巡检，数据量受控）。
func (r *BatchRepo) ListAll(ctx context.Context, q store.Queryer) ([]domain.Batch, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+batchCols+` FROM batches ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Batch
	for rows.Next() {
		b, err := scanBatchRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *b)
	}
	return items, rows.Err()
}
