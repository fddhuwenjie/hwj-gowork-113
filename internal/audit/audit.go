// Package audit 在业务事务内写入审计日志，审计与业务写入同生共死。
package audit

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"germplasm/internal/apperr"
	"germplasm/internal/clock"
	"germplasm/internal/store"
)

// Entry 为一条审计记录。
type Entry struct {
	Actor      string    `json:"actor"`
	Action     string    `json:"action"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	Detail     string    `json:"detail"`
	CreatedAt  time.Time `json:"created_at"`
}

// Writer 将审计日志写入 audit_logs 表。
type Writer struct{}

// NewWriter 创建审计写入器。
func NewWriter() *Writer { return &Writer{} }

// Log 在当前事务中写入审计日志；detail 会序列化为 JSON。
func (w *Writer) Log(ctx context.Context, q store.Queryer, actor, action, entityType, entityID string, detail any, now time.Time) error {
	raw := "{}"
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		raw = string(b)
	}
	_, err := q.ExecContext(ctx, `INSERT INTO audit_logs (actor, action, entity_type, entity_id, detail, created_at)
		VALUES (?,?,?,?,?,?)`, actor, action, entityType, entityID, raw, clock.Format(now))
	return err
}

// List 稳定分页查询审计日志，游标为上一页最后一条自增 ID。
func (w *Writer) List(ctx context.Context, q store.Queryer, entityType, entityID, cursor string, limit int) ([]Entry, string, error) {
	var afterID int64
	if cursor != "" {
		v, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil || v < 0 {
			return nil, "", apperr.Validation("审计分页游标非法")
		}
		afterID = v
	}
	var args []any
	where := " WHERE id > ?"
	args = append(args, afterID)
	if entityType != "" {
		where += " AND entity_type = ?"
		args = append(args, entityType)
	}
	if entityID != "" {
		where += " AND entity_id = ?"
		args = append(args, entityID)
	}
	rows, err := q.QueryContext(ctx, `SELECT id, actor, action, entity_type, entity_id, detail, created_at FROM audit_logs`+where+
		` ORDER BY id LIMIT ?`, append(args, limit+1)...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var entries []Entry
	var ids []int64
	for rows.Next() {
		var e Entry
		var id int64
		var createdAt string
		if err := rows.Scan(&id, &e.Actor, &e.Action, &e.EntityType, &e.EntityID, &e.Detail, &createdAt); err != nil {
			return nil, "", err
		}
		e.CreatedAt = clock.MustParse(createdAt)
		entries = append(entries, e)
		ids = append(ids, id)
	}
	next := ""
	if len(entries) > limit {
		entries = entries[:limit]
		next = itoa(ids[limit-1])
	}
	return entries, next, rows.Err()
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
