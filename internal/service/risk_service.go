package service

import (
	"context"
	"time"

	"germplasm/internal/domain"
	"germplasm/internal/repository"
)

// RiskService 负责风险巡检查询：库位风险、库存差异、连续发芽率下降、
// 待回存批次与谱系异常。
type RiskService struct {
	base baseService
}

// LocationRisk 描述一个冷库的环境风险。
type LocationRisk struct {
	Chamber         string                `json:"chamber"`
	OutOfRangeCount int                   `json:"out_of_range_count"`
	OpenAlerts      int                   `json:"open_alerts"`
	LatestTemp      *domain.SensorReading `json:"latest_temp,omitempty"`
	LatestHumidity  *domain.SensorReading `json:"latest_humidity,omitempty"`
}

// LocationRisks 巡检全部冷库最近 24 小时的越限读数与未处理告警。
func (s *RiskService) LocationRisks(ctx context.Context) ([]LocationRisk, error) {
	chambers, err := s.base.repos.Locations.ListChambers(ctx, s.base.tx.DB())
	if err != nil {
		return nil, err
	}
	sensorPage, err := s.base.repos.Sensors.ListSensors(ctx, s.base.tx.DB(), "", "", 200)
	if err != nil {
		return nil, err
	}
	chamberSensors := map[string][]domain.Sensor{}
	for _, sn := range sensorPage.Items {
		chamberSensors[sn.Chamber] = append(chamberSensors[sn.Chamber], sn)
		if chambers == nil || !containsStr(chambers, sn.Chamber) {
			chambers = append(chambers, sn.Chamber)
		}
	}
	now := s.base.now()
	since := now.Add(-24 * time.Hour)
	var out []LocationRisk
	for _, chamber := range chambers {
		risk := LocationRisk{Chamber: chamber}
		for _, metric := range []domain.SensorMetric{domain.MetricTemperature, domain.MetricHumidity} {
			readings, err := s.base.repos.Sensors.ReadingsInWindow(ctx, s.base.tx.DB(), chamber, metric, since, now)
			if err != nil {
				return nil, err
			}
			rules, err := s.base.repos.Rules.ListActive(ctx, s.base.tx.DB())
			if err != nil {
				return nil, err
			}
			for _, rd := range readings {
				for _, rule := range rules {
					inRange := rule.TempInRange(rd.Value)
					if metric == domain.MetricHumidity {
						inRange = rule.HumidityInRange(rd.Value)
					}
					if !inRange {
						risk.OutOfRangeCount++
						break
					}
				}
			}
		}
		for _, sn := range chamberSensors[chamber] {
			latest, err := s.base.repos.Sensors.LatestReading(ctx, s.base.tx.DB(), sn.ID)
			if err != nil {
				return nil, err
			}
			if latest == nil {
				continue
			}
			if sn.Metric == domain.MetricTemperature {
				risk.LatestTemp = latest
			} else {
				risk.LatestHumidity = latest
			}
		}
		alerts, err := s.base.repos.Alerts.List(ctx, s.base.tx.DB(), "OPEN", domain.AlertEnvOutOfRange, "", 200)
		if err != nil {
			return nil, err
		}
		for _, a := range alerts.Items {
			if a.RefID == chamber {
				risk.OpenAlerts++
			}
		}
		if risk.OutOfRangeCount > 0 || risk.OpenAlerts > 0 {
			out = append(out, risk)
		}
	}
	return out, nil
}

// InventoryVariance 描述批次账面数量与样本汇总之间的差异。
type InventoryVariance struct {
	BatchID       string `json:"batch_id"`
	BatchNo       string `json:"batch_no"`
	QtyTotal      int64  `json:"qty_total"`
	BookSum       int64  `json:"book_sum"`   // 账面分项合计
	SampleSum     int64  `json:"sample_sum"` // 样本汇总
	AvailableDiff int64  `json:"available_diff"`
	Conserved     bool   `json:"conserved"`
}

// InventoryVariances 巡检全部批次的数量守恒与账实差异。
// available_diff 为批次账面可用量与纯在库样本量之差；冻结量单列在 QtyFrozen，
// 不参与可用量核对，合法冻结不得制造可用量差异。
func (s *RiskService) InventoryVariances(ctx context.Context) ([]InventoryVariance, error) {
	batches, err := s.base.repos.Batches.ListAll(ctx, s.base.tx.DB())
	if err != nil {
		return nil, err
	}
	var out []InventoryVariance
	for _, b := range batches {
		sums, err := s.base.repos.Samples.SumByBatchAndStatus(ctx, s.base.tx.DB(), b.ID)
		if err != nil {
			return nil, err
		}
		inStockSum := sums[domain.SampleInStock]
		frozenSum := sums[domain.SampleFrozen]
		outboundSum := sums[domain.SampleOutbound]
		destroyedSum := sums[domain.SampleDestroyed]
		bookSum := b.QtyAvailable + b.QtyFrozen + b.QtyOutbound + b.QtyDestroyed
		sampleSum := inStockSum + frozenSum + outboundSum + destroyedSum
		// 可用量差异：账面可用量与纯在库样本量逐一对应，冻结不重复扣算。
		availableDiff := b.QtyAvailable - inStockSum
		v := InventoryVariance{
			BatchID:       b.ID,
			BatchNo:       b.BatchNo,
			QtyTotal:      b.QtyTotal,
			BookSum:       bookSum,
			SampleSum:     sampleSum,
			AvailableDiff: availableDiff,
			Conserved:     b.CheckConservation() == 0,
		}
		if !v.Conserved || v.AvailableDiff != 0 {
			out = append(out, v)
		}
	}
	return out, nil
}

// GerminationDecline 描述连续发芽率下降的繁育计划。
type GerminationDecline struct {
	PlanID           string    `json:"plan_id"`
	PlanNo           string    `json:"plan_no"`
	ConsecutiveDrops int       `json:"consecutive_drops"`
	Rates            []float64 `json:"rates"`
}

// GerminationDeclines 巡检连续 2 次及以上发芽率下降的繁育计划。
func (s *RiskService) GerminationDeclines(ctx context.Context) ([]GerminationDecline, error) {
	plans, err := s.base.repos.Breeding.ListAllPlans(ctx, s.base.tx.DB())
	if err != nil {
		return nil, err
	}
	var out []GerminationDecline
	for _, p := range plans {
		obs, err := s.base.repos.Breeding.ListObservations(ctx, s.base.tx.DB(), p.ID)
		if err != nil {
			return nil, err
		}
		if len(obs) < 2 {
			continue
		}
		rates := make([]float64, 0, len(obs))
		for _, o := range obs {
			rates = append(rates, o.GerminationRate)
		}
		if drops := domain.ConsecutiveDeclines(rates); drops >= 2 {
			out = append(out, GerminationDecline{PlanID: p.ID, PlanNo: p.PlanNo, ConsecutiveDrops: drops, Rates: rates})
		}
	}
	return out, nil
}

// PendingRestocks 查询待回存验收批次（分页）。
func (s *RiskService) PendingRestocks(ctx context.Context, cursor string, limit int) (*repository.Page[domain.RestockBatch], error) {
	return s.base.repos.Restock.List(ctx, s.base.tx.DB(), string(domain.RestockPending), cursor, repository.NormalizeLimit(limit))
}

// LineageAnomalies 委托谱系服务检测异常。
func (s *RiskService) LineageAnomalies(ctx context.Context) ([]repository.Anomaly, error) {
	edges, err := s.base.repos.Lineage.ListAll(ctx, s.base.tx.DB())
	if err != nil {
		return nil, err
	}
	batches, err := s.base.repos.Batches.ListAll(ctx, s.base.tx.DB())
	if err != nil {
		return nil, err
	}
	hasParent := map[string]bool{}
	for _, e := range edges {
		hasParent[e.ChildBatchID] = true
	}
	var orphans []domain.Batch
	for _, b := range batches {
		if (b.Kind == domain.BatchRestock || b.Kind == domain.BatchRegeneration) && !hasParent[b.ID] {
			orphans = append(orphans, b)
		}
	}
	return s.base.repos.Lineage.DetectAnomalies(edges, orphans), nil
}

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
