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

// ResourceRepo 负责资源与 accession 的持久化。
type ResourceRepo struct{}

// NewResourceRepo 创建仓储。
func NewResourceRepo() *ResourceRepo { return &ResourceRepo{} }

// InsertResource 插入资源。
func (r *ResourceRepo) InsertResource(ctx context.Context, q store.Queryer, res *domain.Resource) error {
	_, err := q.ExecContext(ctx, `INSERT INTO resources
		(id, code, name, species, category, status, remark, version, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		res.ID, res.Code, res.Name, res.Species, res.Category, string(res.Status), res.Remark,
		res.Version, clock.Format(res.CreatedAt), clock.Format(res.UpdatedAt))
	if isUniqueViolation(err) {
		return apperr.Conflict("资源编码已存在: " + res.Code)
	}
	return err
}

// GetResource 按 ID 查询资源。
func (r *ResourceRepo) GetResource(ctx context.Context, q store.Queryer, id string) (*domain.Resource, error) {
	row := q.QueryRowContext(ctx, `SELECT id, code, name, species, category, status, remark, version, created_at, updated_at
		FROM resources WHERE id = ?`, id)
	return scanResource(row)
}

// GetResourceForUpdate 在事务内按 ID 加写锁语义查询资源（SQLite 事务本身串行）。
func (r *ResourceRepo) GetResourceForUpdate(ctx context.Context, q store.Queryer, id string) (*domain.Resource, error) {
	return r.GetResource(ctx, q, id)
}

func scanResource(row *sql.Row) (*domain.Resource, error) {
	var res domain.Resource
	var status, createdAt, updatedAt string
	err := row.Scan(&res.ID, &res.Code, &res.Name, &res.Species, &res.Category, &status,
		&res.Remark, &res.Version, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("资源", "")
	}
	if err != nil {
		return nil, err
	}
	res.Status = domain.ResourceStatus(status)
	res.CreatedAt = clock.MustParse(createdAt)
	res.UpdatedAt = clock.MustParse(updatedAt)
	return &res, nil
}

// UpdateResource 使用乐观锁更新资源。
func (r *ResourceRepo) UpdateResource(ctx context.Context, q store.Queryer, res *domain.Resource, expectedVersion int64, now time.Time) error {
	res.UpdatedAt = now.UTC()
	res.Version = expectedVersion + 1
	r2, err := q.ExecContext(ctx, `UPDATE resources SET name=?, species=?, category=?, status=?, remark=?,
		version=?, updated_at=? WHERE id=? AND version=?`,
		res.Name, res.Species, res.Category, string(res.Status), res.Remark,
		res.Version, clock.Format(res.UpdatedAt), res.ID, expectedVersion)
	if err != nil {
		return err
	}
	return ensureOneRow(r2, "资源", res.ID)
}

// ListResources 稳定分页查询资源。
func (r *ResourceRepo) ListResources(ctx context.Context, q store.Queryer, cursor string, limit int) (*Page[domain.Resource], error) {
	c, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	cond, cursorArgs := cursorCondition(c)
	var args []any
	rows, err := q.QueryContext(ctx, `SELECT id, code, name, species, category, status, remark, version, created_at, updated_at
		FROM resources WHERE 1=1`+cond+` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Resource
	for rows.Next() {
		var res domain.Resource
		var status, createdAt, updatedAt string
		if err := rows.Scan(&res.ID, &res.Code, &res.Name, &res.Species, &res.Category, &status,
			&res.Remark, &res.Version, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		res.Status = domain.ResourceStatus(status)
		res.CreatedAt = clock.MustParse(createdAt)
		res.UpdatedAt = clock.MustParse(updatedAt)
		items = append(items, res)
	}
	return paginate(items, limit, func(r domain.Resource) (time.Time, string) { return r.CreatedAt, r.ID })
}

// InsertAccession 插入 accession。
func (r *ResourceRepo) InsertAccession(ctx context.Context, q store.Queryer, a *domain.Accession) error {
	_, err := q.ExecContext(ctx, `INSERT INTO accessions
		(id, resource_id, accession_no, origin, donor, collected_at, status, version, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.ResourceID, a.AccessionNo, a.Origin, a.Donor, a.CollectedAt, string(a.Status),
		a.Version, clock.Format(a.CreatedAt), clock.Format(a.UpdatedAt))
	if isUniqueViolation(err) {
		return apperr.Conflict("种质编号已存在: " + a.AccessionNo)
	}
	return err
}

// GetAccession 按 ID 查询 accession。
func (r *ResourceRepo) GetAccession(ctx context.Context, q store.Queryer, id string) (*domain.Accession, error) {
	row := q.QueryRowContext(ctx, `SELECT id, resource_id, accession_no, origin, donor, collected_at, status, version, created_at, updated_at
		FROM accessions WHERE id = ?`, id)
	var a domain.Accession
	var status, createdAt, updatedAt string
	err := row.Scan(&a.ID, &a.ResourceID, &a.AccessionNo, &a.Origin, &a.Donor, &a.CollectedAt,
		&status, &a.Version, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("accession", id)
	}
	if err != nil {
		return nil, err
	}
	a.Status = domain.AccessionStatus(status)
	a.CreatedAt = clock.MustParse(createdAt)
	a.UpdatedAt = clock.MustParse(updatedAt)
	return &a, nil
}

// UpdateAccessionStatus 乐观锁更新 accession 状态。
func (r *ResourceRepo) UpdateAccessionStatus(ctx context.Context, q store.Queryer, id string, status domain.AccessionStatus, expectedVersion int64, now time.Time) error {
	r2, err := q.ExecContext(ctx, `UPDATE accessions SET status=?, version=version+1, updated_at=? WHERE id=? AND version=?`,
		string(status), clock.Format(now), id, expectedVersion)
	if err != nil {
		return err
	}
	return ensureOneRow(r2, "accession", id)
}

// ListAccessions 稳定分页查询 accession，可按资源过滤。
func (r *ResourceRepo) ListAccessions(ctx context.Context, q store.Queryer, resourceID, cursor string, limit int) (*Page[domain.Accession], error) {
	c, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	cond, cursorArgs := cursorCondition(c)
	var args []any
	where := " WHERE 1=1"
	// 过滤条件必须随游标推进到每一页：游标只推进 (created_at, id) 的排序位置，
	// 不能清除 resource_id 的隔离，否则跨页会混入其他资源的登记记录。
	if resourceID != "" {
		where += " AND resource_id = ?"
		args = append(args, resourceID)
	}
	rows, err := q.QueryContext(ctx, `SELECT id, resource_id, accession_no, origin, donor, collected_at, status, version, created_at, updated_at
		FROM accessions`+where+cond+` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Accession
	for rows.Next() {
		var a domain.Accession
		var status, createdAt, updatedAt string
		if err := rows.Scan(&a.ID, &a.ResourceID, &a.AccessionNo, &a.Origin, &a.Donor, &a.CollectedAt,
			&status, &a.Version, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		a.Status = domain.AccessionStatus(status)
		a.CreatedAt = clock.MustParse(createdAt)
		a.UpdatedAt = clock.MustParse(updatedAt)
		items = append(items, a)
	}
	return paginate(items, limit, func(a domain.Accession) (time.Time, string) { return a.CreatedAt, a.ID })
}

// paginate 截取分页结果并生成下一页游标。
func paginate[T any](items []T, limit int, key func(T) (time.Time, string)) (*Page[T], error) {
	p := &Page[T]{}
	if len(items) > limit {
		p.Items = items[:limit]
		t, id := key(items[limit-1])
		p.NextCursor = EncodeCursor(t, id)
	} else {
		p.Items = items
	}
	if p.Items == nil {
		p.Items = []T{}
	}
	return p, nil
}

// ensureOneRow 校验乐观锁更新影响了恰好一行。
func ensureOneRow(r sql.Result, entity, id string) error {
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.OptimisticLock(entity, id)
	}
	return nil
}

// isUniqueViolation 判断 SQLite 唯一约束冲突。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "UNIQUE constraint failed") || contains(msg, "constraint failed")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
