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

// SensorRepo 负责传感器与读数持久化。
type SensorRepo struct{}

// NewSensorRepo 创建仓储。
func NewSensorRepo() *SensorRepo { return &SensorRepo{} }

// InsertSensor 插入传感器。
func (r *SensorRepo) InsertSensor(ctx context.Context, q store.Queryer, s *domain.Sensor) error {
	_, err := q.ExecContext(ctx, `INSERT INTO sensors (id, code, chamber, metric, status, created_at) VALUES (?,?,?,?,?,?)`,
		s.ID, s.Code, s.Chamber, string(s.Metric), string(s.Status), clock.Format(s.CreatedAt))
	if isUniqueViolation(err) {
		return apperr.Conflict("传感器编码已存在: " + s.Code)
	}
	return err
}

// GetSensor 按 ID 查询传感器。
func (r *SensorRepo) GetSensor(ctx context.Context, q store.Queryer, id string) (*domain.Sensor, error) {
	row := q.QueryRowContext(ctx, `SELECT id, code, chamber, metric, status, created_at FROM sensors WHERE id = ?`, id)
	var s domain.Sensor
	var metric, status, createdAt string
	err := row.Scan(&s.ID, &s.Code, &s.Chamber, &metric, &status, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("传感器", id)
	}
	if err != nil {
		return nil, err
	}
	s.Metric = domain.SensorMetric(metric)
	s.Status = domain.SensorStatus(status)
	s.CreatedAt = clock.MustParse(createdAt)
	return &s, nil
}

// ListSensors 稳定分页查询传感器。
func (r *SensorRepo) ListSensors(ctx context.Context, q store.Queryer, chamber, cursor string, limit int) (*Page[domain.Sensor], error) {
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
	rows, err := q.QueryContext(ctx, `SELECT id, code, chamber, metric, status, created_at FROM sensors`+where+cond+
		` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Sensor
	for rows.Next() {
		var s domain.Sensor
		var metric, status, createdAt string
		if err := rows.Scan(&s.ID, &s.Code, &s.Chamber, &metric, &status, &createdAt); err != nil {
			return nil, err
		}
		s.Metric = domain.SensorMetric(metric)
		s.Status = domain.SensorStatus(status)
		s.CreatedAt = clock.MustParse(createdAt)
		items = append(items, s)
	}
	return paginate(items, limit, func(s domain.Sensor) (time.Time, string) { return s.CreatedAt, s.ID })
}

// ListSensorsByChamber 查询冷库中指定度量的在线传感器。
func (r *SensorRepo) ListSensorsByChamber(ctx context.Context, q store.Queryer, chamber string, metric domain.SensorMetric) ([]domain.Sensor, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, code, chamber, metric, status, created_at FROM sensors
		WHERE chamber = ? AND metric = ? AND status = 'ONLINE' ORDER BY created_at, id`, chamber, string(metric))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Sensor
	for rows.Next() {
		var s domain.Sensor
		var m, st, createdAt string
		if err := rows.Scan(&s.ID, &s.Code, &s.Chamber, &m, &st, &createdAt); err != nil {
			return nil, err
		}
		s.Metric = domain.SensorMetric(m)
		s.Status = domain.SensorStatus(st)
		s.CreatedAt = clock.MustParse(createdAt)
		items = append(items, s)
	}
	return items, rows.Err()
}

// InsertReading 写入一条环境读数并返回自增 ID。
func (r *SensorRepo) InsertReading(ctx context.Context, q store.Queryer, rd *domain.SensorReading) error {
	res, err := q.ExecContext(ctx, `INSERT INTO sensor_readings (sensor_id, metric, value, recorded_at, created_at) VALUES (?,?,?,?,?)`,
		rd.SensorID, string(rd.Metric), rd.Value, clock.Format(rd.RecordedAt), clock.Format(rd.CreatedAt))
	if err != nil {
		return err
	}
	rd.ID, err = res.LastInsertId()
	return err
}

// ReadingsInWindow 查询冷库在 [start, end] 内的指定度量读数（经传感器关联）。
func (r *SensorRepo) ReadingsInWindow(ctx context.Context, q store.Queryer, chamber string, metric domain.SensorMetric, start, end time.Time) ([]domain.SensorReading, error) {
	rows, err := q.QueryContext(ctx, `SELECT r.id, r.sensor_id, r.metric, r.value, r.recorded_at, r.created_at
		FROM sensor_readings r JOIN sensors s ON s.id = r.sensor_id
		WHERE (s.chamber = ? OR ? = '') AND r.metric = ? AND r.recorded_at >= ? AND r.recorded_at <= ?
		ORDER BY r.recorded_at, r.id`, chamber, chamber, string(metric), clock.Format(start), clock.Format(end))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReadings(rows)
}

// LatestReading 查询传感器最新一条读数。
func (r *SensorRepo) LatestReading(ctx context.Context, q store.Queryer, sensorID string) (*domain.SensorReading, error) {
	row := q.QueryRowContext(ctx, `SELECT id, sensor_id, metric, value, recorded_at, created_at
		FROM sensor_readings WHERE sensor_id = ? ORDER BY recorded_at DESC, id DESC LIMIT 1`, sensorID)
	var rd domain.SensorReading
	var metric, recordedAt, createdAt string
	err := row.Scan(&rd.ID, &rd.SensorID, &metric, &rd.Value, &recordedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rd.Metric = domain.SensorMetric(metric)
	rd.RecordedAt = clock.MustParse(recordedAt)
	rd.CreatedAt = clock.MustParse(createdAt)
	return &rd, nil
}

// ListReadings 稳定分页查询读数。
func (r *SensorRepo) ListReadings(ctx context.Context, q store.Queryer, sensorID string, cursor string, limit int) (*Page[domain.SensorReading], error) {
	c, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	cond, cursorArgs := cursorCondition(c)
	var args []any
	where := " WHERE 1=1"
	if sensorID != "" {
		where += " AND sensor_id = ?"
		args = append(args, sensorID)
	}
	rows, err := q.QueryContext(ctx, `SELECT id, sensor_id, metric, value, recorded_at, created_at FROM sensor_readings`+where+cond+
		` ORDER BY created_at, id LIMIT ?`, append(append(args, cursorArgs...), limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanReadings(rows)
	if err != nil {
		return nil, err
	}
	return paginate(items, limit, func(rd domain.SensorReading) (time.Time, string) {
		return rd.CreatedAt, clock.Format(rd.RecordedAt) + "|" + rd.SensorID
	})
}

func scanReadings(rows *sql.Rows) ([]domain.SensorReading, error) {
	var items []domain.SensorReading
	for rows.Next() {
		var rd domain.SensorReading
		var metric, recordedAt, createdAt string
		if err := rows.Scan(&rd.ID, &rd.SensorID, &metric, &rd.Value, &recordedAt, &createdAt); err != nil {
			return nil, err
		}
		rd.Metric = domain.SensorMetric(metric)
		rd.RecordedAt = clock.MustParse(recordedAt)
		rd.CreatedAt = clock.MustParse(createdAt)
		items = append(items, rd)
	}
	return items, rows.Err()
}
