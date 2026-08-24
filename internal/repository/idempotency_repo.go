package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"germplasm/internal/apperr"
	"germplasm/internal/clock"
	"germplasm/internal/store"
)

// IdempotencyRepo 负责幂等键的存取。每个业务入口以独立 endpoint 标识，
// 拥有相互隔离的幂等命名空间：同一 (key, endpoint) 重复提交且请求体
// 一致时直接返回首个响应；请求体不一致时报幂等冲突。同一 key 可分别
// 用于不同入口（如 purity.create 与 restock.create）而互不冲突，各自
// 保持自己的重复提交语义。
type IdempotencyRepo struct{}

// NewIdempotencyRepo 创建仓储。
func NewIdempotencyRepo() *IdempotencyRepo { return &IdempotencyRepo{} }

// HashRequest 计算请求体的稳定哈希。
func HashRequest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Lookup 查询幂等记录；不存在返回 nil。endpoint 原样作为命名空间的
// 一部分，不同业务入口互不重叠，杜绝跨入口碰撞。
func (r *IdempotencyRepo) Lookup(ctx context.Context, q store.Queryer, key, endpoint string) (requestHash, response string, statusCode int, err error) {
	row := q.QueryRowContext(ctx, `SELECT request_hash, response, status_code FROM idempotency_keys WHERE key=? AND endpoint=?`, key, endpoint)
	err = row.Scan(&requestHash, &response, &statusCode)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", 0, nil
	}
	return requestHash, response, statusCode, err
}

// Save 在同一事务内保存幂等记录。
func (r *IdempotencyRepo) Save(ctx context.Context, q store.Queryer, key, endpoint, requestHash, response string, statusCode int, now time.Time) error {
	_, err := q.ExecContext(ctx, `INSERT INTO idempotency_keys (key, endpoint, request_hash, response, status_code, created_at)
		VALUES (?,?,?,?,?,?)`, key, endpoint, requestHash, response, statusCode, clock.Format(now))
	if isUniqueViolation(err) {
		return apperr.Idempotency(key)
	}
	return err
}
