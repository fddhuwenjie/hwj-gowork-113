package domain

import "time"

// SampleStatus 样本状态。
type SampleStatus string

const (
	SampleInStock   SampleStatus = "IN_STOCK"  // 在库
	SampleFrozen    SampleStatus = "FROZEN"    // 被出库申请冻结
	SampleOutbound  SampleStatus = "OUTBOUND"  // 已出库
	SampleDestroyed SampleStatus = "DESTROYED" // 已销毁
)

// Sample 为批次分装后的最小库存单位，存放在具体库位。
type Sample struct {
	ID         string       `json:"id"`
	BatchID    string       `json:"batch_id"`
	SampleNo   string       `json:"sample_no"`
	Qty        int64        `json:"qty"`
	Status     SampleStatus `json:"status"`
	LocationID string       `json:"location_id,omitempty"`
	Version    int64        `json:"version"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

// LocationStatus 库位状态。
type LocationStatus string

const (
	LocationIdle     LocationStatus = "IDLE"     // 空闲
	LocationActive   LocationStatus = "ACTIVE"   // 使用中
	LocationDisabled LocationStatus = "DISABLED" // 停用
)

// Location 为低温库中的最小库位（冷库/架/盒/格）。
type Location struct {
	ID        string         `json:"id"`
	Code      string         `json:"code"` // 库位编码，全局唯一，如 C01-R02-B03-S04
	Chamber   string         `json:"chamber"`
	Rack      string         `json:"rack"`
	Box       string         `json:"box"`
	Slot      string         `json:"slot"`
	Capacity  int64          `json:"capacity"` // 可容纳样本数
	Occupied  int64          `json:"occupied"` // 已占用样本数
	Status    LocationStatus `json:"status"`
	Version   int64          `json:"version"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// SensorMetric 传感器度量类型。
type SensorMetric string

const (
	MetricTemperature SensorMetric = "TEMPERATURE" // 温度（摄氏度）
	MetricHumidity    SensorMetric = "HUMIDITY"    // 相对湿度（百分比）
)

// SensorStatus 传感器状态。
type SensorStatus string

const (
	SensorOnline  SensorStatus = "ONLINE"
	SensorOffline SensorStatus = "OFFLINE"
)

// Sensor 为部署在某个冷库中的环境传感器。
type Sensor struct {
	ID        string       `json:"id"`
	Code      string       `json:"code"`
	Chamber   string       `json:"chamber"`
	Metric    SensorMetric `json:"metric"`
	Status    SensorStatus `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
}

// SensorReading 为一次环境读数。
type SensorReading struct {
	ID         int64        `json:"id"`
	SensorID   string       `json:"sensor_id"`
	Metric     SensorMetric `json:"metric"`
	Value      float64      `json:"value"`
	RecordedAt time.Time    `json:"recorded_at"`
	CreatedAt  time.Time    `json:"created_at"`
}

// RuleStatus 保存规则版本状态。
type RuleStatus string

const (
	RuleDraft   RuleStatus = "DRAFT"   // 草稿
	RuleActive  RuleStatus = "ACTIVE"  // 启用中（同一 code 唯一）
	RuleRetired RuleStatus = "RETIRED" // 已退役
)

// RuleVersion 为保存规则的不可变版本，定义温湿度阈值、
// 出库前后环境监控窗口与回存质量门槛。
type RuleVersion struct {
	ID                string     `json:"id"`
	Code              string     `json:"code"`
	VersionNo         int        `json:"version_no"`
	MinTemp           float64    `json:"min_temp"` // 允许最低温度
	MaxTemp           float64    `json:"max_temp"` // 允许最高温度
	MinHumidity       float64    `json:"min_humidity"`
	MaxHumidity       float64    `json:"max_humidity"`
	WindowBeforeHours int        `json:"window_before_hours"` // 出库前监控窗口
	WindowAfterHours  int        `json:"window_after_hours"`  // 出库后监控窗口
	MinCoverage       float64    `json:"min_coverage"`        // 检测覆盖率门槛（0~1）
	MinPurity         float64    `json:"min_purity"`          // 纯度合格率门槛（0~1）
	Status            RuleStatus `json:"status"`
	EffectiveFrom     *time.Time `json:"effective_from,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// TempInRange 判断温度是否在规则允许范围内。
func (r *RuleVersion) TempInRange(v float64) bool { return v >= r.MinTemp && v <= r.MaxTemp }

// HumidityInRange 判断湿度是否在规则允许范围内。
func (r *RuleVersion) HumidityInRange(v float64) bool {
	return v >= r.MinHumidity && v <= r.MaxHumidity
}
