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

// SampleRepo 负责样本持久化。
type SampleRepo struct{}

// NewSampleRepo 创建仓储。
func NewSampleRepo() *SampleRepo { return &SampleRepo{} }

const sampleCols = `id, batch_id, sample_no, qty, status, location_id, version, created_at, updated_at`

// Insert 插入样本。
func (r *SampleRepo) Insert(ctx context.Context, q store.Queryer, s *domain.Sample) error {
	_, err := q.ExecContext(ctx, `INSERT INTO samples (`+sampleCols+`) VALUES (?,?,?,?,?,?,?,?,?)`,
		s.ID, s.BatchID, s.SampleNo, s.Qty, string(s.Status), s.LocationID,
		s.Version, clock.Format(s.CreatedAt), clock.Format(s.UpdatedAt))
	if isUniqueViolation(err) {
		return apperr.Conflict("样本编号已存在: " + s.SampleNo)
	}
	return err
}

// Get 按 ID 查询样本。
func (r *SampleRepo) Get(ctx context.Context, q store.Queryer, id string) (*domain.Sample, error) {
	row := q.QueryRowContext(ctx, `SELECT `+sampleCols+` FROM samples WHERE id = ?`, id)
	return scanSample(row)
}

func scanSample(row *sql.Row) (*domain.Sample, error) {
	var s domain.Sample
	var status, createdAt, updatedAt string
	err := row.Scan(&s.ID, &s.BatchID, &s.SampleNo, &s.Qty, &status, &s.LocationID,
		&s.Version, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("样本", "")
	}
	if err != nil {
		return nil, err
	}
	s.Status = domain.SampleStatus(status)
	s.CreatedAt = clock.MustParse(createdAt)
	s.UpdatedAt = clock.MustParse(updatedAt)
	return &s, nil
}

// Update 乐观锁更新样本。
func (r *SampleRepo) Update(ctx context.Context, q store.Queryer, s *domain.Sample, expectedVersion int64, now time.Time) error {
	r2, err := q.ExecContext(ctx, `UPDATE samples SET qty=?, status=?, location_id=?, version=version+1, updated_at=?
		WHERE id=? AND version=?`,
		s.Qty, string(s.Status), s.LocationID, clock.Format(now), s.ID, expectedVersion)
	if err != nil {
		return err
	}
	if err := ensureOneRow(r2, "样本", s.ID); err != nil {
		return err
	}
	s.Version = expectedVersion + 1
	s.UpdatedAt = now
	return nil
}

// ListByBatch 查询批次下指定状态的样本，按创建时间升序（FIFO）。
func (r *SampleRepo) ListByBatch(ctx context.Context, q store.Queryer, batchID string, status domain.SampleStatus) ([]domain.Sample, error) {
	query := `SELECT ` + sampleCols + ` FROM samples WHERE batch_id = ?`
	args := []any{batchID}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, string(status))
	} else {
		query += ` AND status != 'DESTROYED'`
	}
	query += ` ORDER BY created_at, id`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Sample
	for rows.Next() {
		var s domain.Sample
		var st, createdAt, updatedAt string
		if err := rows.Scan(&s.ID, &s.BatchID, &s.SampleNo, &s.Qty, &st, &s.LocationID,
			&s.Version, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		s.Status = domain.SampleStatus(st)
		s.CreatedAt = clock.MustParse(createdAt)
		s.UpdatedAt = clock.MustParse(updatedAt)
		items = append(items, s)
	}
	return items, rows.Err()
}

// List 稳定分页查询样本。
func (r *SampleRepo) List(ctx context.Context, q store.Queryer, batchID, status, cursor string, limit int) (*Page[domain.Sample], error) {
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
	rows, err := q.QueryContext(ctx, `SELECT `+sampleCols+` FROM samples`+where+cond+
		` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Sample
	for rows.Next() {
		var s domain.Sample
		var st, createdAt, updatedAt string
		if err := rows.Scan(&s.ID, &s.BatchID, &s.SampleNo, &s.Qty, &st, &s.LocationID,
			&s.Version, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		s.Status = domain.SampleStatus(st)
		s.CreatedAt = clock.MustParse(createdAt)
		s.UpdatedAt = clock.MustParse(updatedAt)
		items = append(items, s)
	}
	return paginate(items, limit, func(s domain.Sample) (time.Time, string) { return s.CreatedAt, s.ID })
}

// SumByBatchAndStatus 汇总批次在库/冻结/出库/销毁样本数量，用于守恒巡检。
func (r *SampleRepo) SumByBatchAndStatus(ctx context.Context, q store.Queryer, batchID string) (map[domain.SampleStatus]int64, error) {
	rows, err := q.QueryContext(ctx, `SELECT status, COALESCE(SUM(qty),0) FROM samples WHERE batch_id = ? GROUP BY status`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[domain.SampleStatus]int64{}
	for rows.Next() {
		var st string
		var sum int64
		if err := rows.Scan(&st, &sum); err != nil {
			return nil, err
		}
		out[domain.SampleStatus(st)] = sum
	}
	return out, rows.Err()
}
