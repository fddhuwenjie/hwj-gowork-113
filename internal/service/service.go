// Package service 实现业务用例编排：事务、幂等、乐观锁、快照与审计。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"germplasm/internal/apperr"
	"germplasm/internal/audit"
	"germplasm/internal/clock"
	"germplasm/internal/repository"
	"germplasm/internal/store"
)

// Services 聚合全部领域服务与其依赖。
type Services struct {
	Tx          *store.TxManager
	Clock       clock.Clock
	Audit       *audit.Writer
	Resources   *ResourceService
	Storage     *StorageService
	Sensors     *SensorService
	Rules       *RuleService
	Outbound    *OutboundService
	Breeding    *BreedingService
	Purity      *PurityService
	Restock     *RestockService
	Destruction *DestructionService
	Lineage     *LineageService
	Risk        *RiskService
	// 仓储，供后台作业复用
	Repos *Repos
}

// Repos 聚合全部仓储。
type Repos struct {
	Resources   *repository.ResourceRepo
	Batches     *repository.BatchRepo
	Samples     *repository.SampleRepo
	Locations   *repository.LocationRepo
	Sensors     *repository.SensorRepo
	Rules       *repository.RuleRepo
	Outbound    *repository.OutboundRepo
	Breeding    *repository.BreedingRepo
	Purity      *repository.PurityRepo
	Restock     *repository.RestockRepo
	Destruction *repository.DestructionRepo
	Lineage     *repository.LineageRepo
	Snapshots   *repository.SnapshotRepo
	Idempotency *repository.IdempotencyRepo
	Jobs        *repository.JobRepo
	Alerts      *repository.AlertRepo
}

// NewRepos 创建仓储聚合。
func NewRepos() *Repos {
	return &Repos{
		Resources:   repository.NewResourceRepo(),
		Batches:     repository.NewBatchRepo(),
		Samples:     repository.NewSampleRepo(),
		Locations:   repository.NewLocationRepo(),
		Sensors:     repository.NewSensorRepo(),
		Rules:       repository.NewRuleRepo(),
		Outbound:    repository.NewOutboundRepo(),
		Breeding:    repository.NewBreedingRepo(),
		Purity:      repository.NewPurityRepo(),
		Restock:     repository.NewRestockRepo(),
		Destruction: repository.NewDestructionRepo(),
		Lineage:     repository.NewLineageRepo(),
		Snapshots:   repository.NewSnapshotRepo(),
		Idempotency: repository.NewIdempotencyRepo(),
		Jobs:        repository.NewJobRepo(),
		Alerts:      repository.NewAlertRepo(),
	}
}

// New 组装全部领域服务。
func New(tx *store.TxManager, clk clock.Clock, auditWriter *audit.Writer, repos *Repos) *Services {
	s := &Services{Tx: tx, Clock: clk, Audit: auditWriter, Repos: repos}
	base := baseService{tx: tx, clk: clk, audit: auditWriter, repos: repos}
	s.Resources = &ResourceService{base: base}
	s.Storage = &StorageService{base: base}
	s.Sensors = &SensorService{base: base}
	s.Rules = &RuleService{base: base}
	s.Outbound = &OutboundService{base: base}
	s.Breeding = &BreedingService{base: base}
	s.Purity = &PurityService{base: base}
	s.Restock = &RestockService{base: base}
	s.Destruction = &DestructionService{base: base}
	s.Lineage = &LineageService{base: base}
	s.Risk = &RiskService{base: base}
	return s
}

// baseService 为各服务共享的依赖基座。
type baseService struct {
	tx    *store.TxManager
	clk   clock.Clock
	audit *audit.Writer
	repos *Repos
}

// now 返回注入时钟的当前时间。
func (b *baseService) now() time.Time { return b.clk.Now() }

// idemLookup 在创建前检查幂等键：命中且请求体一致时返回已存实体 ID；
// 命中但请求体不一致时返回幂等冲突错误。
func (b *baseService) idemLookup(ctx context.Context, q store.Queryer, key, endpoint, reqHash string) (entityID string, found bool, err error) {
	if key == "" {
		return "", false, nil
	}
	hash, response, _, err := b.repos.Idempotency.Lookup(ctx, q, key, endpoint)
	if err != nil {
		return "", false, err
	}
	if response == "" {
		return "", false, nil
	}
	if hash != reqHash {
		return "", false, apperr.Idempotency(key)
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(response), &payload); err != nil {
		return "", false, err
	}
	return payload.ID, true, nil
}

// idemSave 在同一事务内保存幂等记录。
func (b *baseService) idemSave(ctx context.Context, q store.Queryer, key, endpoint, reqHash, entityID string) error {
	if key == "" {
		return nil
	}
	response := fmt.Sprintf(`{"id":%q}`, entityID)
	return b.repos.Idempotency.Save(ctx, q, key, endpoint, reqHash, response, 200, b.now())
}

// requirePositive 校验正整数。
func requirePositive(name string, v int64) error {
	if v <= 0 {
		return apperr.Validationf("%s 必须为正数", name)
	}
	return nil
}

// requireNonEmpty 校验非空字符串。
func requireNonEmpty(name, v string) error {
	if v == "" {
		return apperr.Validationf("%s 不能为空", name)
	}
	return nil
}
