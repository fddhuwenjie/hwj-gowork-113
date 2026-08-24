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

// OutboundRepo 负责出库申请与冻结明细持久化。
type OutboundRepo struct{}

// NewOutboundRepo 创建仓储。
func NewOutboundRepo() *OutboundRepo { return &OutboundRepo{} }

const outboundCols = `id, request_no, accession_id, batch_id, qty, purpose, breeding_target, rule_version_id,
	deadline, status, idempotency_key, version, created_at, updated_at`

// Insert 插入出库申请。
func (r *OutboundRepo) Insert(ctx context.Context, q store.Queryer, o *domain.OutboundRequest) error {
	var key *string
	if o.IdempotencyKey != "" {
		key = &o.IdempotencyKey
	}
	_, err := q.ExecContext(ctx, `INSERT INTO outbound_requests (`+outboundCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		o.ID, o.RequestNo, o.AccessionID, o.BatchID, o.Qty, o.Purpose, o.BreedingTarget, o.RuleVersionID,
		clock.Format(o.Deadline), string(o.Status), key, o.Version, clock.Format(o.CreatedAt), clock.Format(o.UpdatedAt))
	if isUniqueViolation(err) {
		return apperr.Conflict("出库申请编号或幂等键已存在")
	}
	return err
}

// Get 按 ID 查询出库申请。
func (r *OutboundRepo) Get(ctx context.Context, q store.Queryer, id string) (*domain.OutboundRequest, error) {
	row := q.QueryRowContext(ctx, `SELECT `+outboundCols+` FROM outbound_requests WHERE id = ?`, id)
	return scanOutbound(row)
}

// GetByIdempotencyKey 按幂等键查询出库申请。
func (r *OutboundRepo) GetByIdempotencyKey(ctx context.Context, q store.Queryer, key string) (*domain.OutboundRequest, error) {
	row := q.QueryRowContext(ctx, `SELECT `+outboundCols+` FROM outbound_requests WHERE idempotency_key = ?`, key)
	o, err := scanOutbound(row)
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) && ae.Code == apperr.CodeNotFound {
			return nil, nil
		}
		return nil, err
	}
	return o, nil
}

func scanOutbound(row *sql.Row) (*domain.OutboundRequest, error) {
	var o domain.OutboundRequest
	var status, deadline, createdAt, updatedAt string
	var key *string
	err := row.Scan(&o.ID, &o.RequestNo, &o.AccessionID, &o.BatchID, &o.Qty, &o.Purpose, &o.BreedingTarget,
		&o.RuleVersionID, &deadline, &status, &key, &o.Version, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("出库申请", "")
	}
	if err != nil {
		return nil, err
	}
	o.Status = domain.OutboundStatus(status)
	o.Deadline = clock.MustParse(deadline)
	o.CreatedAt = clock.MustParse(createdAt)
	o.UpdatedAt = clock.MustParse(updatedAt)
	if key != nil {
		o.IdempotencyKey = *key
	}
	return &o, nil
}

// UpdateStatus 乐观锁更新出库申请状态。
func (r *OutboundRepo) UpdateStatus(ctx context.Context, q store.Queryer, id string, status domain.OutboundStatus, expectedVersion int64, now time.Time) error {
	r2, err := q.ExecContext(ctx, `UPDATE outbound_requests SET status=?, version=version+1, updated_at=? WHERE id=? AND version=?`,
		string(status), clock.Format(now), id, expectedVersion)
	if err != nil {
		return err
	}
	return ensureOneRow(r2, "出库申请", id)
}

// List 稳定分页查询出库申请。
func (r *OutboundRepo) List(ctx context.Context, q store.Queryer, status, cursor string, limit int) (*Page[domain.OutboundRequest], error) {
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
	rows, err := q.QueryContext(ctx, `SELECT `+outboundCols+` FROM outbound_requests`+where+cond+
		` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.OutboundRequest
	for rows.Next() {
		o, err := scanOutboundRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *o)
	}
	return paginate(items, limit, func(o domain.OutboundRequest) (time.Time, string) { return o.CreatedAt, o.ID })
}

func scanOutboundRows(rows *sql.Rows) (*domain.OutboundRequest, error) {
	var o domain.OutboundRequest
	var status, deadline, createdAt, updatedAt string
	var key *string
	if err := rows.Scan(&o.ID, &o.RequestNo, &o.AccessionID, &o.BatchID, &o.Qty, &o.Purpose, &o.BreedingTarget,
		&o.RuleVersionID, &deadline, &status, &key, &o.Version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	o.Status = domain.OutboundStatus(status)
	o.Deadline = clock.MustParse(deadline)
	o.CreatedAt = clock.MustParse(createdAt)
	o.UpdatedAt = clock.MustParse(updatedAt)
	if key != nil {
		o.IdempotencyKey = *key
	}
	return &o, nil
}

// ListApprovedDueBefore 查询截止时间早于 threshold 的已审批申请（临期巡检用）。
// 覆盖本轮全部到期申请，不做截断，由调用方决定后续处理。
func (r *OutboundRepo) ListApprovedDueBefore(ctx context.Context, q store.Queryer, threshold time.Time) ([]domain.OutboundRequest, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+outboundCols+` FROM outbound_requests
		WHERE status='APPROVED' AND deadline <= ? ORDER BY deadline, id`, clock.Format(threshold))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.OutboundRequest
	for rows.Next() {
		o, err := scanOutboundRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *o)
	}
	return items, rows.Err()
}

// InsertFreeze 插入冻结明细。
func (r *OutboundRepo) InsertFreeze(ctx context.Context, q store.Queryer, f *domain.OutboundFreeze) error {
	_, err := q.ExecContext(ctx, `INSERT INTO outbound_freezes (id, request_id, sample_id, location_id, qty, status, created_at)
		VALUES (?,?,?,?,?,?,?)`, f.ID, f.RequestID, f.SampleID, f.LocationID, f.Qty, string(f.Status), clock.Format(f.CreatedAt))
	return err
}

// ListFreezes 查询申请下的冻结明细。
func (r *OutboundRepo) ListFreezes(ctx context.Context, q store.Queryer, requestID string, status domain.FreezeStatus) ([]domain.OutboundFreeze, error) {
	query := `SELECT id, request_id, sample_id, location_id, qty, status, created_at FROM outbound_freezes WHERE request_id = ?`
	args := []any{requestID}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, string(status))
	}
	query += ` ORDER BY created_at, id`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.OutboundFreeze
	for rows.Next() {
		var f domain.OutboundFreeze
		var st, createdAt string
		if err := rows.Scan(&f.ID, &f.RequestID, &f.SampleID, &f.LocationID, &f.Qty, &st, &createdAt); err != nil {
			return nil, err
		}
		f.Status = domain.FreezeStatus(st)
		f.CreatedAt = clock.MustParse(createdAt)
		items = append(items, f)
	}
	return items, rows.Err()
}

// UpdateFreezeStatus 更新冻结明细状态。
func (r *OutboundRepo) UpdateFreezeStatus(ctx context.Context, q store.Queryer, id string, status domain.FreezeStatus) error {
	_, err := q.ExecContext(ctx, `UPDATE outbound_freezes SET status=? WHERE id=?`, string(status), id)
	return err
}
