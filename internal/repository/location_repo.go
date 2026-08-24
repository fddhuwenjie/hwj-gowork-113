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

// LocationRepo 负责低温库位持久化。
type LocationRepo struct{}

// NewLocationRepo 创建仓储。
func NewLocationRepo() *LocationRepo { return &LocationRepo{} }

const locationCols = `id, code, chamber, rack, box, slot, capacity, occupied, status, version, created_at, updated_at`

// Insert 插入库位。
func (r *LocationRepo) Insert(ctx context.Context, q store.Queryer, l *domain.Location) error {
	_, err := q.ExecContext(ctx, `INSERT INTO locations (`+locationCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		l.ID, l.Code, l.Chamber, l.Rack, l.Box, l.Slot, l.Capacity, l.Occupied,
		string(l.Status), l.Version, clock.Format(l.CreatedAt), clock.Format(l.UpdatedAt))
	if isUniqueViolation(err) {
		return apperr.Conflict("库位编码已存在: " + l.Code)
	}
	return err
}

// Get 按 ID 查询库位。
func (r *LocationRepo) Get(ctx context.Context, q store.Queryer, id string) (*domain.Location, error) {
	row := q.QueryRowContext(ctx, `SELECT `+locationCols+` FROM locations WHERE id = ?`, id)
	return scanLocation(row)
}

func scanLocation(row *sql.Row) (*domain.Location, error) {
	var l domain.Location
	var status, createdAt, updatedAt string
	err := row.Scan(&l.ID, &l.Code, &l.Chamber, &l.Rack, &l.Box, &l.Slot,
		&l.Capacity, &l.Occupied, &status, &l.Version, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("库位", "")
	}
	if err != nil {
		return nil, err
	}
	l.Status = domain.LocationStatus(status)
	l.CreatedAt = clock.MustParse(createdAt)
	l.UpdatedAt = clock.MustParse(updatedAt)
	return &l, nil
}

// Occupy 乐观锁占用一个库位容量。
func (r *LocationRepo) Occupy(ctx context.Context, q store.Queryer, id string, expectedVersion int64, now time.Time) error {
	r2, err := q.ExecContext(ctx, `UPDATE locations SET occupied=occupied+1,
		status=CASE WHEN occupied+1 >= capacity THEN 'ACTIVE' ELSE status END,
		version=version+1, updated_at=?
		WHERE id=? AND version=? AND occupied < capacity AND status != 'DISABLED'`,
		clock.Format(now), id, expectedVersion)
	if err != nil {
		return err
	}
	n, err := r2.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.Conflict("库位容量不足或已停用: " + id)
	}
	return nil
}

// Release 乐观锁释放一个库位容量。
func (r *LocationRepo) Release(ctx context.Context, q store.Queryer, id string, expectedVersion int64, now time.Time) error {
	r2, err := q.ExecContext(ctx, `UPDATE locations SET occupied=MAX(occupied-1,0), version=version+1, updated_at=?
		WHERE id=? AND version=?`, clock.Format(now), id, expectedVersion)
	if err != nil {
		return err
	}
	return ensureOneRow(r2, "库位", id)
}

// List 稳定分页查询库位。
func (r *LocationRepo) List(ctx context.Context, q store.Queryer, chamber, cursor string, limit int) (*Page[domain.Location], error) {
	c, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	cond, cursorArgs := cursorCondition(c)
	var args []any
	where := " WHERE 1=1"
	if chamber != "" {
		where += " AND chamber = ?"
		args = append(args, chamber)
	}
	rows, err := q.QueryContext(ctx, `SELECT `+locationCols+` FROM locations`+where+cond+
		` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Location
	for rows.Next() {
		var l domain.Location
		var status, createdAt, updatedAt string
		if err := rows.Scan(&l.ID, &l.Code, &l.Chamber, &l.Rack, &l.Box, &l.Slot,
			&l.Capacity, &l.Occupied, &status, &l.Version, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		l.Status = domain.LocationStatus(status)
		l.CreatedAt = clock.MustParse(createdAt)
		l.UpdatedAt = clock.MustParse(updatedAt)
		items = append(items, l)
	}
	return paginate(items, limit, func(l domain.Location) (time.Time, string) { return l.CreatedAt, l.ID })
}

// ListChambers 返回全部冷库编号。
func (r *LocationRepo) ListChambers(ctx context.Context, q store.Queryer) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT DISTINCT chamber FROM locations ORDER BY chamber`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
