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

// BreedingRepo 负责繁育计划与田间观察持久化。
type BreedingRepo struct{}

// NewBreedingRepo 创建仓储。
func NewBreedingRepo() *BreedingRepo { return &BreedingRepo{} }

const planCols = `id, plan_no, outbound_request_id, batch_id, target_qty, plot, deadline, status, version, created_at, updated_at`

// InsertPlan 插入繁育计划。
func (r *BreedingRepo) InsertPlan(ctx context.Context, q store.Queryer, p *domain.BreedingPlan) error {
	_, err := q.ExecContext(ctx, `INSERT INTO breeding_plans (`+planCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.PlanNo, p.OutboundRequestID, p.BatchID, p.TargetQty, p.Plot,
		clock.Format(p.Deadline), string(p.Status), p.Version, clock.Format(p.CreatedAt), clock.Format(p.UpdatedAt))
	if isUniqueViolation(err) {
		return apperr.Conflict("繁育计划编号已存在或该出库申请已建立繁育计划")
	}
	return err
}

// GetPlan 按 ID 查询繁育计划。
func (r *BreedingRepo) GetPlan(ctx context.Context, q store.Queryer, id string) (*domain.BreedingPlan, error) {
	row := q.QueryRowContext(ctx, `SELECT `+planCols+` FROM breeding_plans WHERE id = ?`, id)
	return scanPlan(row)
}

func scanPlan(row *sql.Row) (*domain.BreedingPlan, error) {
	var p domain.BreedingPlan
	var status, deadline, createdAt, updatedAt string
	err := row.Scan(&p.ID, &p.PlanNo, &p.OutboundRequestID, &p.BatchID, &p.TargetQty, &p.Plot,
		&deadline, &status, &p.Version, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("繁育计划", "")
	}
	if err != nil {
		return nil, err
	}
	p.Status = domain.PlanStatus(status)
	p.Deadline = clock.MustParse(deadline)
	p.CreatedAt = clock.MustParse(createdAt)
	p.UpdatedAt = clock.MustParse(updatedAt)
	return &p, nil
}

// UpdatePlanStatus 乐观锁更新繁育计划状态。
func (r *BreedingRepo) UpdatePlanStatus(ctx context.Context, q store.Queryer, id string, status domain.PlanStatus, expectedVersion int64, now time.Time) error {
	r2, err := q.ExecContext(ctx, `UPDATE breeding_plans SET status=?, version=version+1, updated_at=? WHERE id=? AND version=?`,
		string(status), clock.Format(now), id, expectedVersion)
	if err != nil {
		return err
	}
	return ensureOneRow(r2, "繁育计划", id)
}

// ListPlans 稳定分页查询繁育计划。
func (r *BreedingRepo) ListPlans(ctx context.Context, q store.Queryer, status, cursor string, limit int) (*Page[domain.BreedingPlan], error) {
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
	rows, err := q.QueryContext(ctx, `SELECT `+planCols+` FROM breeding_plans`+where+cond+
		` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.BreedingPlan
	for rows.Next() {
		p, err := scanPlanRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *p)
	}
	return paginate(items, limit, func(p domain.BreedingPlan) (time.Time, string) { return p.CreatedAt, p.ID })
}

func scanPlanRows(rows *sql.Rows) (*domain.BreedingPlan, error) {
	var p domain.BreedingPlan
	var status, deadline, createdAt, updatedAt string
	if err := rows.Scan(&p.ID, &p.PlanNo, &p.OutboundRequestID, &p.BatchID, &p.TargetQty, &p.Plot,
		&deadline, &status, &p.Version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	p.Status = domain.PlanStatus(status)
	p.Deadline = clock.MustParse(deadline)
	p.CreatedAt = clock.MustParse(createdAt)
	p.UpdatedAt = clock.MustParse(updatedAt)
	return &p, nil
}

// ListActiveExpired 查询已超过繁育期限的 ACTIVE 计划（超时巡检用）。
func (r *BreedingRepo) ListActiveExpired(ctx context.Context, q store.Queryer, now time.Time) ([]domain.BreedingPlan, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+planCols+` FROM breeding_plans WHERE status='ACTIVE' AND deadline < ? ORDER BY deadline, id`,
		clock.Format(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.BreedingPlan
	for rows.Next() {
		p, err := scanPlanRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *p)
	}
	return items, rows.Err()
}

// ListAllPlans 查询全部计划（风险分析用）。
func (r *BreedingRepo) ListAllPlans(ctx context.Context, q store.Queryer) ([]domain.BreedingPlan, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+planCols+` FROM breeding_plans ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.BreedingPlan
	for rows.Next() {
		p, err := scanPlanRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *p)
	}
	return items, rows.Err()
}

// InsertObservation 插入田间观察。
func (r *BreedingRepo) InsertObservation(ctx context.Context, q store.Queryer, o *domain.FieldObservation) error {
	_, err := q.ExecContext(ctx, `INSERT INTO field_observations (id, plan_id, observed_at, germination_rate, vigor, notes, created_at)
		VALUES (?,?,?,?,?,?,?)`, o.ID, o.PlanID, clock.Format(o.ObservedAt), o.GerminationRate, o.Vigor, o.Notes, clock.Format(o.CreatedAt))
	return err
}

// ListObservations 按观察时间升序查询计划的田间观察。
func (r *BreedingRepo) ListObservations(ctx context.Context, q store.Queryer, planID string) ([]domain.FieldObservation, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, plan_id, observed_at, germination_rate, vigor, notes, created_at
		FROM field_observations WHERE plan_id = ? ORDER BY observed_at, id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.FieldObservation
	for rows.Next() {
		var o domain.FieldObservation
		var observedAt, createdAt string
		if err := rows.Scan(&o.ID, &o.PlanID, &observedAt, &o.GerminationRate, &o.Vigor, &o.Notes, &createdAt); err != nil {
			return nil, err
		}
		o.ObservedAt = clock.MustParse(observedAt)
		o.CreatedAt = clock.MustParse(createdAt)
		items = append(items, o)
	}
	return items, rows.Err()
}
