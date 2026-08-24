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

// JobRepo 负责持久化后台作业，支撑失败重试与重启恢复。
type JobRepo struct{}

// NewJobRepo 创建仓储。
func NewJobRepo() *JobRepo { return &JobRepo{} }

const jobCols = `id, type, payload, status, attempts, max_attempts, next_run_at, last_error, version, created_at, updated_at`

// Enqueue 入队作业；同类型且未完成的扫描类作业不重复入队。
func (r *JobRepo) Enqueue(ctx context.Context, q store.Queryer, j *domain.Job) error {
	_, err := q.ExecContext(ctx, `INSERT INTO jobs (`+jobCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		j.ID, j.Type, j.Payload, j.Status, j.Attempts, j.MaxAttempts, clock.Format(j.NextRunAt), j.LastError,
		j.Version, clock.Format(j.CreatedAt), clock.Format(j.UpdatedAt))
	return err
}

// EnqueueIfAbsent 仅当同类型无 PENDING/RUNNING 作业时入队，用于周期扫描去重。
func (r *JobRepo) EnqueueIfAbsent(ctx context.Context, q store.Queryer, j *domain.Job) error {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(1) FROM jobs WHERE type=? AND status IN ('PENDING','RUNNING')`, j.Type).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return r.Enqueue(ctx, q, j)
}

// Claim 抢占一个到期作业：PENDING -> RUNNING，乐观锁防并发重复执行。
func (r *JobRepo) Claim(ctx context.Context, q store.Queryer, now time.Time) (*domain.Job, error) {
	row := q.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE status='PENDING' AND next_run_at <= ? ORDER BY next_run_at, id LIMIT 1`,
		clock.Format(now))
	j, err := scanJob(row)
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) && ae.Code == apperr.CodeNotFound {
			return nil, nil // 无到期作业
		}
		return nil, err
	}
	r2, err := q.ExecContext(ctx, `UPDATE jobs SET status='RUNNING', attempts=attempts+1, version=version+1, updated_at=?
		WHERE id=? AND version=? AND status='PENDING'`, clock.Format(now), j.ID, j.Version)
	if err != nil {
		return nil, err
	}
	n, err := r2.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil // 被其他执行者抢占
	}
	j.Status = domain.JobRunning
	j.Attempts++
	j.Version++
	return j, nil
}

// Complete 标记作业完成。
func (r *JobRepo) Complete(ctx context.Context, q store.Queryer, id string, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE jobs SET status='DONE', version=version+1, updated_at=? WHERE id=?`,
		clock.Format(now), id)
	return err
}

// Fail 标记作业失败；未超过最大尝试次数时按指数退避重新排队。
func (r *JobRepo) Fail(ctx context.Context, q store.Queryer, j *domain.Job, cause error, now time.Time) error {
	if j.Attempts >= j.MaxAttempts {
		_, err := q.ExecContext(ctx, `UPDATE jobs SET status='FAILED', last_error=?, version=version+1, updated_at=? WHERE id=?`,
			cause.Error(), clock.Format(now), j.ID)
		return err
	}
	backoff := time.Duration(1<<uint(j.Attempts-1)) * time.Second
	if backoff > time.Minute {
		backoff = time.Minute
	}
	_, err := q.ExecContext(ctx, `UPDATE jobs SET status='PENDING', last_error=?, next_run_at=?, version=version+1, updated_at=? WHERE id=?`,
		cause.Error(), clock.Format(now.Add(backoff)), clock.Format(now), j.ID)
	return err
}

// RecoverStuck 重启恢复：将崩溃遗留的 RUNNING 作业重置为 PENDING。
func (r *JobRepo) RecoverStuck(ctx context.Context, q store.Queryer, now time.Time) (int64, error) {
	r2, err := q.ExecContext(ctx, `UPDATE jobs SET status='PENDING', version=version+1, updated_at=? WHERE status='RUNNING'`,
		clock.Format(now))
	if err != nil {
		return 0, err
	}
	return r2.RowsAffected()
}

// Get 按 ID 查询作业。
func (r *JobRepo) Get(ctx context.Context, q store.Queryer, id string) (*domain.Job, error) {
	row := q.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE id = ?`, id)
	return scanJob(row)
}

func scanJob(row *sql.Row) (*domain.Job, error) {
	var j domain.Job
	var nextRunAt, createdAt, updatedAt string
	err := row.Scan(&j.ID, &j.Type, &j.Payload, &j.Status, &j.Attempts, &j.MaxAttempts, &nextRunAt, &j.LastError,
		&j.Version, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("作业", "")
	}
	if err != nil {
		return nil, err
	}
	j.NextRunAt = clock.MustParse(nextRunAt)
	j.CreatedAt = clock.MustParse(createdAt)
	j.UpdatedAt = clock.MustParse(updatedAt)
	return &j, nil
}

// List 稳定分页查询作业。
func (r *JobRepo) List(ctx context.Context, q store.Queryer, status, cursor string, limit int) (*Page[domain.Job], error) {
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
	rows, err := q.QueryContext(ctx, `SELECT `+jobCols+` FROM jobs`+where+cond+
		` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Job
	for rows.Next() {
		var j domain.Job
		var nextRunAt, createdAt, updatedAt string
		if err := rows.Scan(&j.ID, &j.Type, &j.Payload, &j.Status, &j.Attempts, &j.MaxAttempts, &nextRunAt, &j.LastError,
			&j.Version, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		j.NextRunAt = clock.MustParse(nextRunAt)
		j.CreatedAt = clock.MustParse(createdAt)
		j.UpdatedAt = clock.MustParse(updatedAt)
		items = append(items, j)
	}
	return paginate(items, limit, func(j domain.Job) (time.Time, string) { return j.CreatedAt, j.ID })
}
