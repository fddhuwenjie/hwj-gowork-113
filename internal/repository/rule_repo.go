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

// RuleRepo 负责保存规则版本持久化。
type RuleRepo struct{}

// NewRuleRepo 创建仓储。
func NewRuleRepo() *RuleRepo { return &RuleRepo{} }

const ruleCols = `id, code, version_no, min_temp, max_temp, min_humidity, max_humidity,
	window_before_hours, window_after_hours, min_coverage, min_purity, status, effective_from, created_at`

// Insert 插入规则版本。
func (r *RuleRepo) Insert(ctx context.Context, q store.Queryer, rv *domain.RuleVersion) error {
	var eff *string
	if rv.EffectiveFrom != nil {
		s := clock.Format(*rv.EffectiveFrom)
		eff = &s
	}
	_, err := q.ExecContext(ctx, `INSERT INTO rule_versions (`+ruleCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		rv.ID, rv.Code, rv.VersionNo, rv.MinTemp, rv.MaxTemp, rv.MinHumidity, rv.MaxHumidity,
		rv.WindowBeforeHours, rv.WindowAfterHours, rv.MinCoverage, rv.MinPurity,
		string(rv.Status), eff, clock.Format(rv.CreatedAt))
	if isUniqueViolation(err) {
		return apperr.Newf(409, apperr.CodeConflict, "规则 %s 版本 %d 已存在", rv.Code, rv.VersionNo)
	}
	return err
}

// Get 按 ID 查询规则版本。
func (r *RuleRepo) Get(ctx context.Context, q store.Queryer, id string) (*domain.RuleVersion, error) {
	row := q.QueryRowContext(ctx, `SELECT `+ruleCols+` FROM rule_versions WHERE id = ?`, id)
	return scanRule(row)
}

func scanRule(row *sql.Row) (*domain.RuleVersion, error) {
	var rv domain.RuleVersion
	var status, createdAt string
	var eff *string
	err := row.Scan(&rv.ID, &rv.Code, &rv.VersionNo, &rv.MinTemp, &rv.MaxTemp, &rv.MinHumidity, &rv.MaxHumidity,
		&rv.WindowBeforeHours, &rv.WindowAfterHours, &rv.MinCoverage, &rv.MinPurity, &status, &eff, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("保存规则", "")
	}
	if err != nil {
		return nil, err
	}
	rv.Status = domain.RuleStatus(status)
	rv.EffectiveFrom = parseNullTime(eff)
	rv.CreatedAt = clock.MustParse(createdAt)
	return &rv, nil
}

// NextVersionNo 计算规则 code 的下一个版本号。
func (r *RuleRepo) NextVersionNo(ctx context.Context, q store.Queryer, code string) (int, error) {
	var n sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT MAX(version_no) FROM rule_versions WHERE code = ?`, code).Scan(&n)
	if err != nil {
		return 0, err
	}
	if !n.Valid {
		return 1, nil
	}
	return int(n.Int64) + 1, nil
}

// Activate 在事务内启用规则版本：先校验目标版本处于 DRAFT（退役为终态，
// 不可重新启用），再将同 code 的旧 ACTIVE 版本全部退役，最后置目标版本为 ACTIVE。
// 任何非法转换都返回 STATE_CONFLICT 并随事务回滚，保证当前生效版本不被破坏。
func (r *RuleRepo) Activate(ctx context.Context, q store.Queryer, id string, now time.Time) error {
	rv, err := r.Get(ctx, q, id)
	if err != nil {
		return err
	}
	if err := domain.MustTransition("rule", string(rv.Status), string(domain.RuleActive)); err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `UPDATE rule_versions SET status='RETIRED' WHERE code=? AND status='ACTIVE'`, rv.Code); err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `UPDATE rule_versions SET status='ACTIVE', effective_from=? WHERE id=? AND status='DRAFT'`,
		clock.Format(now), id); err != nil {
		return err
	}
	return nil
}

// ActiveByCode 查询 code 当前启用版本。
func (r *RuleRepo) ActiveByCode(ctx context.Context, q store.Queryer, code string) (*domain.RuleVersion, error) {
	row := q.QueryRowContext(ctx, `SELECT `+ruleCols+` FROM rule_versions WHERE code=? AND status='ACTIVE'`, code)
	return scanRule(row)
}

// ListActive 查询全部启用中的规则版本。
func (r *RuleRepo) ListActive(ctx context.Context, q store.Queryer) ([]domain.RuleVersion, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+ruleCols+` FROM rule_versions WHERE status='ACTIVE' ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.RuleVersion
	for rows.Next() {
		rv, err := scanRuleRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *rv)
	}
	return items, rows.Err()
}

// List 稳定分页查询规则版本。
func (r *RuleRepo) List(ctx context.Context, q store.Queryer, code, cursor string, limit int) (*Page[domain.RuleVersion], error) {
	c, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	cond, cursorArgs := cursorCondition(c)
	var args []any
	where := " WHERE 1=1"
	if code != "" {
		where += " AND code = ?"
		args = append(args, code)
	}
	rows, err := q.QueryContext(ctx, `SELECT `+ruleCols+` FROM rule_versions`+where+cond+
		` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.RuleVersion
	for rows.Next() {
		rv, err := scanRuleRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *rv)
	}
	return paginate(items, limit, func(rv domain.RuleVersion) (time.Time, string) { return rv.CreatedAt, rv.ID })
}

func scanRuleRows(rows *sql.Rows) (*domain.RuleVersion, error) {
	var rv domain.RuleVersion
	var status, createdAt string
	var eff *string
	if err := rows.Scan(&rv.ID, &rv.Code, &rv.VersionNo, &rv.MinTemp, &rv.MaxTemp, &rv.MinHumidity, &rv.MaxHumidity,
		&rv.WindowBeforeHours, &rv.WindowAfterHours, &rv.MinCoverage, &rv.MinPurity, &status, &eff, &createdAt); err != nil {
		return nil, err
	}
	rv.Status = domain.RuleStatus(status)
	rv.EffectiveFrom = parseNullTime(eff)
	rv.CreatedAt = clock.MustParse(createdAt)
	return &rv, nil
}
