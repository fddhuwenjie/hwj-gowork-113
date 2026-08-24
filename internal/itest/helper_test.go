// Package itest 使用真实临时 SQLite 文件对完整业务链进行集成测试。
package itest

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"germplasm/internal/apperr"
	"germplasm/internal/audit"
	"germplasm/internal/clock"
	"germplasm/internal/config"
	"germplasm/internal/domain"
	"germplasm/internal/jobs"
	"germplasm/internal/service"
	"germplasm/internal/store"
)

// testEnv 汇总测试环境依赖。
type testEnv struct {
	t      *testing.T
	ctx    context.Context
	db     *store.DB
	clk    *clock.Fake
	svc    *service.Services
	sched  *jobs.Scheduler
	dbPath string
	seq    int
}

// newTestEnv 在临时目录创建真实 SQLite 文件并组装服务。
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	start := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	clk := clock.NewFake(start)
	dbPath := filepath.Join(t.TempDir(), "itest.db")
	db, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	txMgr := store.NewTxManager(db)
	svc := service.New(txMgr, clk, audit.NewWriter(), service.NewRepos())
	cfg := config.Config{
		DBPath:               dbPath,
		JobMaxAttempts:       3,
		OutboundDueSoonHours: 24,
		RestockPendingHours:  72,
	}
	sched := jobs.NewScheduler(svc, cfg, clk, testLogger())
	return &testEnv{
		t:      t,
		ctx:    context.Background(),
		db:     db,
		clk:    clk,
		svc:    svc,
		sched:  sched,
		dbPath: dbPath,
	}
}

// reopen 关闭并重新打开同一数据库文件，模拟进程重启。
func (e *testEnv) reopen() {
	e.t.Helper()
	if err := e.db.Close(); err != nil {
		e.t.Fatalf("关闭数据库失败: %v", err)
	}
	db, err := store.Open(context.Background(), e.dbPath)
	if err != nil {
		e.t.Fatalf("重新打开数据库失败: %v", err)
	}
	e.db = db
	e.t.Cleanup(func() { db.Close() })
	txMgr := store.NewTxManager(db)
	e.svc = service.New(txMgr, e.clk, audit.NewWriter(), service.NewRepos())
	cfg := config.Config{DBPath: e.dbPath, JobMaxAttempts: 3, OutboundDueSoonHours: 24, RestockPendingHours: 72}
	e.sched = jobs.NewScheduler(e.svc, cfg, e.clk, testLogger())
}

// unique 生成测试内唯一编号。
func (e *testEnv) unique(prefix string) string {
	e.seq++
	return fmt.Sprintf("%s-%04d", prefix, e.seq)
}

// setupBase 完成资源登记、批次建立、样本分装与库位分配，返回核心实体。
func (e *testEnv) setupBase(qty int64, sampleQty int64, chamber string) (*domain.Resource, *domain.Accession, *domain.Batch, *domain.Location) {
	e.t.Helper()
	ctx := e.ctx
	res, err := e.svc.Resources.CreateResource(ctx, "tester", service.CreateResourceInput{
		Code: e.unique("RES"), Name: "水稻种质", Species: "Oryza sativa", Category: "粮食作物",
	})
	if err != nil {
		e.t.Fatalf("创建资源失败: %v", err)
	}
	acc, err := e.svc.Resources.CreateAccession(ctx, "tester", service.CreateAccessionInput{
		ResourceID: res.ID, AccessionNo: e.unique("ACC"), Origin: "云南", Donor: "某农科院",
	})
	if err != nil {
		e.t.Fatalf("创建 accession 失败: %v", err)
	}
	batch, err := e.svc.Storage.CreateOriginalBatch(ctx, "tester", service.CreateBatchInput{
		AccessionID: acc.ID, BatchNo: e.unique("BAT"), QtyTotal: qty,
	})
	if err != nil {
		e.t.Fatalf("创建批次失败: %v", err)
	}
	loc, err := e.svc.Storage.CreateLocation(ctx, "tester", service.CreateLocationInput{
		Code: e.unique("C01-R01-B01-S"), Chamber: chamber, Rack: "R01", Box: "B01", Slot: "S01", Capacity: 100,
	})
	if err != nil {
		e.t.Fatalf("创建库位失败: %v", err)
	}
	var quantities []int64
	remaining := qty
	for remaining > 0 {
		q := sampleQty
		if remaining < q {
			q = remaining
		}
		quantities = append(quantities, q)
		remaining -= q
	}
	samples, err := e.svc.Storage.SplitSamples(ctx, "tester", service.SplitSamplesInput{BatchID: batch.ID, Quantities: quantities})
	if err != nil {
		e.t.Fatalf("样本分装失败: %v", err)
	}
	for _, smp := range samples {
		if _, err := e.svc.Storage.AssignLocation(ctx, "tester", smp.ID, loc.ID, smp.Version); err != nil {
			e.t.Fatalf("分配库位失败: %v", err)
		}
	}
	return res, acc, batch, loc
}

// setupRule 创建并启用保存规则。
func (e *testEnv) setupRule() *domain.RuleVersion {
	e.t.Helper()
	rv, err := e.svc.Rules.CreateRuleVersion(e.ctx, "tester", service.CreateRuleInput{
		Code:              "STD-SEED",
		MinTemp:           -20,
		MaxTemp:           -15,
		MinHumidity:       20,
		MaxHumidity:       40,
		WindowBeforeHours: 2,
		WindowAfterHours:  2,
		MinCoverage:       0.9,
		MinPurity:         0.95,
	})
	if err != nil {
		e.t.Fatalf("创建规则失败: %v", err)
	}
	activated, err := e.svc.Rules.ActivateRule(e.ctx, "tester", rv.ID)
	if err != nil {
		e.t.Fatalf("启用规则失败: %v", err)
	}
	return activated
}

// setupSensors 注册温湿度传感器并写入覆盖指定窗口的达标读数。
func (e *testEnv) setupSensors(chamber string, windowHours int) {
	e.t.Helper()
	e.setupSensorsWithValue(chamber, windowHours, -18, 30)
}

// setupSensorsWithValue 按指定温湿度值写入覆盖窗口的读数（每小时桶一条）。
func (e *testEnv) setupSensorsWithValue(chamber string, windowHours int, temp, humidity float64) {
	e.t.Helper()
	tempSensor, err := e.svc.Sensors.CreateSensor(e.ctx, "tester", service.CreateSensorInput{
		Code: e.unique("TEMP"), Chamber: chamber, Metric: "TEMPERATURE",
	})
	if err != nil {
		e.t.Fatalf("创建温度传感器失败: %v", err)
	}
	humSensor, err := e.svc.Sensors.CreateSensor(e.ctx, "tester", service.CreateSensorInput{
		Code: e.unique("HUM"), Chamber: chamber, Metric: "HUMIDITY",
	})
	if err != nil {
		e.t.Fatalf("创建湿度传感器失败: %v", err)
	}
	now := e.clk.Now()
	for h := windowHours; h >= 0; h-- {
		at := now.Add(-time.Duration(h) * time.Hour)
		if h == 0 {
			at = now.Add(-time.Minute) // 当前时刻的读数略提前，避免落在窗口外
		}
		for _, pair := range []struct {
			id    string
			value float64
		}{{tempSensor.ID, temp}, {humSensor.ID, humidity}} {
			_, err := e.svc.Sensors.AddReading(e.ctx, pair.id, service.AddReadingInput{
				Value: pair.value, RecordedAt: at.Format(time.RFC3339Nano),
			})
			if err != nil {
				e.t.Fatalf("写入环境读数失败: %v", err)
			}
		}
	}
}

// mustErrCode 断言错误为指定应用错误码。
func mustErrCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望错误码 %s，但无错误", code)
	}
	ae, ok := err.(*apperr.Error)
	if !ok {
		t.Fatalf("期望应用错误 %s，实际为 %T: %v", code, err, err)
	}
	if ae.Code != code {
		t.Fatalf("期望错误码 %s，实际 %s: %v", code, ae.Code, ae)
	}
}

// newRuleInput 生成一条自定义规则入参（默认不启用）。
func ruleInput(code string) service.CreateRuleInput {
	return service.CreateRuleInput{
		Code:              code,
		MinTemp:           -20,
		MaxTemp:           -15,
		MinHumidity:       20,
		MaxHumidity:       40,
		WindowBeforeHours: 2,
		WindowAfterHours:  2,
		MinCoverage:       1.0,
		MinPurity:         0.95,
	}
}
