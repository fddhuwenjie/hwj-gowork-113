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

// PurityRepo 负责纯度检测持久化。
type PurityRepo struct{}

// NewPurityRepo 创建仓储。
func NewPurityRepo() *PurityRepo { return &PurityRepo{} }

const purityCols = `id, plan_id, sample_qty, coverage_ratio, purity_rate, verdict, sealed, sealed_at, tested_at, idempotency_key, version, created_at`

// Insert 插入纯度检测。
func (r *PurityRepo) Insert(ctx context.Context, q store.Queryer, t *domain.PurityTest) error {
	var sealedAt, key *string
	if t.SealedAt != nil {
		s := clock.Format(*t.SealedAt)
		sealedAt = &s
	}
	if t.IdempotencyKey != "" {
		key = &t.IdempotencyKey
	}
	sealed := 0
	if t.Sealed {
		sealed = 1
	}
	_, err := q.ExecContext(ctx, `INSERT INTO purity_tests (`+purityCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.PlanID, t.SampleQty, t.CoverageRatio, t.PurityRate, string(t.Verdict), sealed, sealedAt,
		clock.Format(t.TestedAt), key, t.Version, clock.Format(t.CreatedAt))
	if isUniqueViolation(err) {
		return apperr.Conflict("检测幂等键已存在")
	}
	return err
}

// Get 按 ID 查询检测。
func (r *PurityRepo) Get(ctx context.Context, q store.Queryer, id string) (*domain.PurityTest, error) {
	row := q.QueryRowContext(ctx, `SELECT `+purityCols+` FROM purity_tests WHERE id = ?`, id)
	return scanPurity(row)
}

// GetByIdempotencyKey 按幂等键查询检测，不存在时返回 nil, nil。
func (r *PurityRepo) GetByIdempotencyKey(ctx context.Context, q store.Queryer, key string) (*domain.PurityTest, error) {
	row := q.QueryRowContext(ctx, `SELECT `+purityCols+` FROM purity_tests WHERE idempotency_key = ?`, key)
	t, err := scanPurity(row)
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) && ae.Code == apperr.CodeNotFound {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func scanPurity(row *sql.Row) (*domain.PurityTest, error) {
	var t domain.PurityTest
	var verdict, testedAt, createdAt string
	var sealedAt, key *string
	var sealed int
	err := row.Scan(&t.ID, &t.PlanID, &t.SampleQty, &t.CoverageRatio, &t.PurityRate, &verdict, &sealed, &sealedAt,
		&testedAt, &key, &t.Version, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("纯度检测", "")
	}
	if err != nil {
		return nil, err
	}
	t.Verdict = domain.TestVerdict(verdict)
	t.Sealed = sealed == 1
	t.SealedAt = parseNullTime(sealedAt)
	t.TestedAt = clock.MustParse(testedAt)
	if key != nil {
		t.IdempotencyKey = *key
	}
	t.CreatedAt = clock.MustParse(createdAt)
	return &t, nil
}

// Seal 乐观锁封存检测结论。
func (r *PurityRepo) Seal(ctx context.Context, q store.Queryer, id string, verdict domain.TestVerdict, expectedVersion int64, now time.Time) error {
	r2, err := q.ExecContext(ctx, `UPDATE purity_tests SET verdict=?, sealed=1, sealed_at=?, version=version+1
		WHERE id=? AND version=? AND sealed=0`,
		string(verdict), clock.Format(now), id, expectedVersion)
	if err != nil {
		return err
	}
	n, err := r2.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.Statef("检测 %s 已封存或版本不匹配", id)
	}
	return nil
}

// LatestSealed 查询计划最新一条已封存检测。
func (r *PurityRepo) LatestSealed(ctx context.Context, q store.Queryer, planID string) (*domain.PurityTest, error) {
	row := q.QueryRowContext(ctx, `SELECT `+purityCols+` FROM purity_tests
		WHERE plan_id=? AND sealed=1 ORDER BY sealed_at DESC, id DESC LIMIT 1`, planID)
	t, err := scanPurity(row)
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) && ae.Code == apperr.CodeNotFound {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

// List 稳定分页查询检测。
func (r *PurityRepo) List(ctx context.Context, q store.Queryer, planID, cursor string, limit int) (*Page[domain.PurityTest], error) {
	c, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	cond, cursorArgs := cursorCondition(c)
	var args []any
	where := " WHERE 1=1"
	if planID != "" {
		where += " AND plan_id = ?"
		args = append(args, planID)
	}
	rows, err := q.QueryContext(ctx, `SELECT `+purityCols+` FROM purity_tests`+where+cond+
		` ORDER BY tested_at DESC, id DESC LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.PurityTest
	for rows.Next() {
		var t domain.PurityTest
		var verdict, testedAt, createdAt string
		var sealedAt, key *string
		var sealed int
		if err := rows.Scan(&t.ID, &t.PlanID, &t.SampleQty, &t.CoverageRatio, &t.PurityRate, &verdict, &sealed, &sealedAt,
			&testedAt, &key, &t.Version, &createdAt); err != nil {
			return nil, err
		}
		t.Verdict = domain.TestVerdict(verdict)
		t.Sealed = sealed == 1
		t.SealedAt = parseNullTime(sealedAt)
		t.TestedAt = clock.MustParse(testedAt)
		if key != nil {
			t.IdempotencyKey = *key
		}
		t.CreatedAt = clock.MustParse(createdAt)
		items = append(items, t)
	}
	return paginate(items, limit, func(t domain.PurityTest) (time.Time, string) { return t.CreatedAt, t.ID })
}
