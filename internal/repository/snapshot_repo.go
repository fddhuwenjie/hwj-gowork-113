package repository

import (
	"context"
	"encoding/json"
	"time"

	"germplasm/internal/clock"
	"germplasm/internal/domain"
	"germplasm/internal/store"
)

// SnapshotRepo 负责历史快照持久化，快照只增不改。
type SnapshotRepo struct{}

// NewSnapshotRepo 创建仓储。
func NewSnapshotRepo() *SnapshotRepo { return &SnapshotRepo{} }

// Add 在同一事务内追加一条历史快照。
func (r *SnapshotRepo) Add(ctx context.Context, q store.Queryer, entityType, entityID, event string, payload any, now time.Time) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO snapshots (entity_type, entity_id, event, payload, created_at) VALUES (?,?,?,?,?)`,
		entityType, entityID, event, string(raw), clock.Format(now))
	return err
}

// List 稳定分页查询快照。
func (r *SnapshotRepo) List(ctx context.Context, q store.Queryer, entityType, entityID, cursor string, limit int) (*Page[domain.Snapshot], error) {
	c, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	cond, cursorArgs := cursorCondition(c)
	var args []any
	where := " WHERE 1=1"
	if entityType != "" {
		where += " AND entity_type = ?"
		args = append(args, entityType)
	}
	if entityID != "" {
		where += " AND entity_id = ?"
		args = append(args, entityID)
	}
	rows, err := q.QueryContext(ctx, `SELECT id, entity_type, entity_id, event, payload, created_at FROM snapshots`+where+cond+
		` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Snapshot
	for rows.Next() {
		var s domain.Snapshot
		var createdAt string
		if err := rows.Scan(&s.ID, &s.EntityType, &s.EntityID, &s.Event, &s.Payload, &createdAt); err != nil {
			return nil, err
		}
		s.CreatedAt = clock.MustParse(createdAt)
		items = append(items, s)
	}
	return paginate(items, limit, func(s domain.Snapshot) (time.Time, string) {
		return s.CreatedAt, s.EntityType + "|" + s.EntityID
	})
}
