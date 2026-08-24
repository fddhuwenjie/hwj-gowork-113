package service

import (
	"context"
	"database/sql"
	"time"

	"germplasm/internal/apperr"
	"germplasm/internal/domain"
	"germplasm/internal/repository"
)

// SensorService 负责环境传感器与读数采集。
type SensorService struct {
	base baseService
}

// CreateSensorInput 传感器注册入参。
type CreateSensorInput struct {
	Code    string `json:"code"`
	Chamber string `json:"chamber"`
	Metric  string `json:"metric"` // TEMPERATURE / HUMIDITY
}

// CreateSensor 注册环境传感器。
func (s *SensorService) CreateSensor(ctx context.Context, actor string, in CreateSensorInput) (*domain.Sensor, error) {
	if err := requireNonEmpty("传感器编码", in.Code); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("冷库编号", in.Chamber); err != nil {
		return nil, err
	}
	metric := domain.SensorMetric(in.Metric)
	if metric != domain.MetricTemperature && metric != domain.MetricHumidity {
		return nil, apperr.Validation("度量类型必须为 TEMPERATURE 或 HUMIDITY")
	}
	now := s.base.now()
	sensor := &domain.Sensor{
		ID:        domain.NewID(domain.PrefixSensor),
		Code:      in.Code,
		Chamber:   in.Chamber,
		Metric:    metric,
		Status:    domain.SensorOnline,
		CreatedAt: now,
	}
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		if err := s.base.repos.Sensors.InsertSensor(ctx, tx, sensor); err != nil {
			return err
		}
		return s.base.audit.Log(ctx, tx, actor, "sensor.create", "sensor", sensor.ID, sensor, now)
	})
	if err != nil {
		return nil, err
	}
	return sensor, nil
}

// ListSensors 分页查询传感器。
func (s *SensorService) ListSensors(ctx context.Context, chamber, cursor string, limit int) (*repository.Page[domain.Sensor], error) {
	if cursor != "" {
		chamber = ""
	}
	return s.base.repos.Sensors.ListSensors(ctx, s.base.tx.DB(), chamber, cursor, repository.NormalizeLimit(limit))
}

// AddReadingInput 环境读数入参。
type AddReadingInput struct {
	Value      float64 `json:"value"`
	RecordedAt string  `json:"recorded_at"` // RFC3339，缺省取当前时间
}

// AddReading 为传感器追加一条环境读数。读数只增不改。
func (s *SensorService) AddReading(ctx context.Context, sensorID string, in AddReadingInput) (*domain.SensorReading, error) {
	now := s.base.now()
	recordedAt := now
	if in.RecordedAt != "" {
		t, err := time.Parse(time.RFC3339Nano, in.RecordedAt)
		if err != nil {
			return nil, apperr.Validation("recorded_at 必须为 RFC3339 时间")
		}
		recordedAt = t.UTC()
	}
	var rd *domain.SensorReading
	err := s.base.tx.InTx(ctx, func(tx *sql.Tx) error {
		sensor, err := s.base.repos.Sensors.GetSensor(ctx, tx, sensorID)
		if err != nil {
			return err
		}
		rd = &domain.SensorReading{
			SensorID:   sensor.ID,
			Metric:     sensor.Metric,
			Value:      in.Value,
			RecordedAt: recordedAt,
			CreatedAt:  now,
		}
		return s.base.repos.Sensors.InsertReading(ctx, tx, rd)
	})
	if err != nil {
		return nil, err
	}
	return rd, nil
}

// ListReadings 分页查询环境读数。
func (s *SensorService) ListReadings(ctx context.Context, sensorID, cursor string, limit int) (*repository.Page[domain.SensorReading], error) {
	return s.base.repos.Sensors.ListReadings(ctx, s.base.tx.DB(), sensorID, cursor, repository.NormalizeLimit(limit))
}
