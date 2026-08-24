// Package repository 提供各实体的持久化仓储，基于 database/sql 与 SQLite。
package repository

import (
	"encoding/base64"
	"strings"
	"time"

	"germplasm/internal/apperr"
	"germplasm/internal/clock"
)

// Page 为稳定分页结果，按 (created_at, id) 升序游标翻页。
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// Cursor 解析后的分页游标。
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// EncodeCursor 编码游标。
func EncodeCursor(createdAt time.Time, id string) string {
	raw := clock.Format(createdAt) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor 解码并校验游标。
func DecodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, apperr.Validation("分页游标非法")
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return Cursor{}, apperr.Validation("分页游标格式非法")
	}
	t, err := clock.Parse(parts[0])
	if err != nil {
		return Cursor{}, apperr.Validation("分页游标时间非法")
	}
	return Cursor{CreatedAt: t, ID: parts[1]}, nil
}

// NormalizeLimit 规整分页大小，默认 20，最大 200。
func NormalizeLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 200 {
		return 200
	}
	return limit
}

// cursorCondition 生成稳定分页的 WHERE 片段与参数。
func cursorCondition(c Cursor) (string, []any) {
	if c.ID == "" {
		return "", nil
	}
	return " AND (created_at > ? OR (created_at = ? AND id > ?))",
		[]any{clock.Format(c.CreatedAt), clock.Format(c.CreatedAt), c.ID}
}

// parseNullTime 解析可空时间字段。
func parseNullTime(ns *string) *time.Time {
	if ns == nil || *ns == "" {
		return nil
	}
	t, err := clock.Parse(*ns)
	if err != nil {
		return nil
	}
	return &t
}
