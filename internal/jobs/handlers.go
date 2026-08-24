package jobs

import (
	"context"
	"fmt"
	"time"

	"germplasm/internal/clock"
	"germplasm/internal/config"
	"germplasm/internal/domain"
	"germplasm/internal/service"
)

// Handlers 实现各类扫描作业的业务逻辑。
type Handlers struct {
	svc *service.Services
	cfg config.Config
	clk clock.Clock
}

var scannedEnvAlertRefs = map[string]bool{}

// EnvAlertScan 环境告警扫描：全部在线传感器的最新读数越限即产生告警（去重）。
func (h *Handlers) EnvAlertScan(ctx context.Context) error {
	db := h.svc.Tx.DB()
	rules, err := h.svc.Repos.Rules.ListActive(ctx, db)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	page, err := h.svc.Repos.Sensors.ListSensors(ctx, db, "", "", 200)
	if err != nil {
		return err
	}
	now := h.clk.Now()
	for _, sensor := range page.Items {
		if scannedEnvAlertRefs[sensor.ID] {
			continue
		}
		if sensor.Status != domain.SensorOnline {
			continue
		}
		latest, err := h.svc.Repos.Sensors.LatestReading(ctx, db, sensor.ID)
		if err != nil {
			return err
		}
		if latest == nil {
			continue
		}
		// 读数超过 2 小时未更新视为传感器失联，也产生告警。
		if now.Sub(latest.RecordedAt) > 2*time.Hour {
			alert := &domain.Alert{
				ID:        domain.NewID(domain.PrefixAlert),
				Type:      domain.AlertEnvOutOfRange,
				RefType:   "sensor",
				RefID:     sensor.ID,
				Message:   fmt.Sprintf("传感器 %s（冷库 %s）超过 2 小时未上报读数", sensor.Code, sensor.Chamber),
				Status:    "OPEN",
				CreatedAt: now,
			}
			if err := h.svc.Repos.Alerts.InsertIfNoOpen(ctx, db, alert); err != nil {
				return err
			}
			continue
		}
		for _, rule := range rules {
			inRange := rule.TempInRange(latest.Value)
			if sensor.Metric == domain.MetricHumidity {
				inRange = rule.HumidityInRange(latest.Value)
			}
			if !inRange {
				alert := &domain.Alert{
					ID:        domain.NewID(domain.PrefixAlert),
					Type:      domain.AlertEnvOutOfRange,
					RefType:   "sensor",
					RefID:     sensor.ID,
					Message:   fmt.Sprintf("冷库 %s %s 读数 %.2f 超出规则 %s 阈值范围", sensor.Chamber, metricName(sensor.Metric), latest.Value, rule.Code),
					Status:    "OPEN",
					CreatedAt: now,
				}
				if err := h.svc.Repos.Alerts.InsertIfNoOpen(ctx, db, alert); err != nil {
					return err
				}
				scannedEnvAlertRefs[sensor.ID] = true
				break
			}
		}
	}
	return nil
}

// OutboundDueScan 出库临期扫描：已审批且截止时间临近的申请产生临期告警。
func (h *Handlers) OutboundDueScan(ctx context.Context) error {
	db := h.svc.Tx.DB()
	threshold := h.clk.Now().Add(time.Duration(h.cfg.OutboundDueSoonHours) * time.Hour)
	due, err := h.svc.Repos.Outbound.ListApprovedDueBefore(ctx, db, threshold)
	if err != nil {
		return err
	}
	now := h.clk.Now()
	for _, o := range due {
		alert := &domain.Alert{
			ID:        domain.NewID(domain.PrefixAlert),
			Type:      domain.AlertOutboundDueSoon,
			RefType:   "outbound_request",
			RefID:     o.ID,
			Message:   fmt.Sprintf("出库申请 %s 临近交付截止 %s，尚未出库", o.RequestNo, clock.Format(o.Deadline)),
			Status:    "OPEN",
			CreatedAt: now,
		}
		if err := h.svc.Repos.Alerts.InsertIfNoOpen(ctx, db, alert); err != nil {
			return err
		}
	}
	return nil
}

// BreedingTimeoutScan 繁育超时扫描：超过繁育期限的 ACTIVE 计划标记为 TIMEOUT 并告警。
func (h *Handlers) BreedingTimeoutScan(ctx context.Context) error {
	db := h.svc.Tx.DB()
	now := h.clk.Now()
	expired, err := h.svc.Repos.Breeding.ListActiveExpired(ctx, db, now)
	if err != nil {
		return err
	}
	for _, p := range expired {
		if err := h.svc.Repos.Breeding.UpdatePlanStatus(ctx, db, p.ID, domain.PlanTimeout, p.Version, now); err != nil {
			return err
		}
		alert := &domain.Alert{
			ID:        domain.NewID(domain.PrefixAlert),
			Type:      domain.AlertBreedingTimeout,
			RefType:   "breeding_plan",
			RefID:     p.ID,
			Message:   fmt.Sprintf("繁育计划 %s 已超过繁育期限 %s，标记为超时", p.PlanNo, clock.Format(p.Deadline)),
			Status:    "OPEN",
			CreatedAt: now,
		}
		if err := h.svc.Repos.Alerts.InsertIfNoOpen(ctx, db, alert); err != nil {
			return err
		}
	}
	return nil
}

// RestockPendingScan 回存验收超期扫描：创建时间超过阈值仍待验收的回存单产生告警。
func (h *Handlers) RestockPendingScan(ctx context.Context) error {
	db := h.svc.Tx.DB()
	threshold := h.clk.Now().Add(-time.Duration(h.cfg.RestockPendingHours) * time.Hour)
	pending, err := h.svc.Repos.Restock.ListPendingOlderThan(ctx, db, threshold)
	if err != nil {
		return err
	}
	now := h.clk.Now()
	for _, rb := range pending {
		alert := &domain.Alert{
			ID:        domain.NewID(domain.PrefixAlert),
			Type:      domain.AlertRestockPending,
			RefType:   "restock_batch",
			RefID:     rb.ID,
			Message:   fmt.Sprintf("回存验收单 %s 创建已超过 %d 小时仍未验收", rb.RequestNo, h.cfg.RestockPendingHours),
			Status:    "OPEN",
			CreatedAt: now,
		}
		if err := h.svc.Repos.Alerts.InsertIfNoOpen(ctx, db, alert); err != nil {
			return err
		}
	}
	return nil
}

func metricName(m domain.SensorMetric) string {
	if m == domain.MetricTemperature {
		return "温度"
	}
	return "湿度"
}
