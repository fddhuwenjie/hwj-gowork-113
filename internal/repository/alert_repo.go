package repository

import (
	"context"
	"time"

	"germplasm/internal/apperr"
	"germplasm/internal/clock"
	"germplasm/internal/domain"
	"germplasm/internal/store"
)

// AlertRepo 负责告警持久化。
type AlertRepo struct{}

// NewAlertRepo 创建仓储。
func NewAlertRepo() *AlertRepo { return &AlertRepo{} }

// InsertIfNoOpen 仅当同一 (type, ref) 无 OPEN 告警时插入，避免告警风暴。
func (r *AlertRepo) InsertIfNoOpen(ctx context.Context, q store.Queryer, a *domain.Alert) error {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(1) FROM alerts WHERE type=? AND ref_type=? AND ref_id=? AND status='OPEN'`,
		a.Type, a.RefType, a.RefID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := q.ExecContext(ctx, `INSERT INTO alerts (id, type, ref_type, ref_id, message, status, created_at) VALUES (?,?,?,?,?,?,?)`,
		a.ID, a.Type, a.RefType, a.RefID, a.Message, a.Status, clock.Format(a.CreatedAt))
	return err
}

// Ack 确认告警。
func (r *AlertRepo) Ack(ctx context.Context, q store.Queryer, id string, now time.Time) error {
	r2, err := q.ExecContext(ctx, `UPDATE alerts SET status='ACKED', acked_at=? WHERE id=? AND status='OPEN'`, clock.Format(now), id)
	if err != nil {
		return err
	}
	n, err := r2.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.Statef("告警 %s 不存在或已确认", id)
	}
	return nil
}

// List 稳定分页查询告警。
func (r *AlertRepo) List(ctx context.Context, q store.Queryer, status, alertType, cursor string, limit int) (*Page[domain.Alert], error) {
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
	if alertType != "" {
		where += " AND type = ?"
		args = append(args, alertType)
	}
	rows, err := q.QueryContext(ctx, `SELECT id, type, ref_type, ref_id, message, status, created_at, acked_at FROM alerts`+where+cond+
		` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Alert
	for rows.Next() {
		var a domain.Alert
		var createdAt string
		var ackedAt *string
		if err := rows.Scan(&a.ID, &a.Type, &a.RefType, &a.RefID, &a.Message, &a.Status, &createdAt, &ackedAt); err != nil {
			return nil, err
		}
		a.CreatedAt = clock.MustParse(createdAt)
		a.AckedAt = parseNullTime(ackedAt)
		items = append(items, a)
	}
	return paginate(items, limit, func(a domain.Alert) (time.Time, string) { return a.CreatedAt, a.ID })
}
