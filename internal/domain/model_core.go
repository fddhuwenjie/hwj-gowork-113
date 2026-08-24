package domain

import "time"

// ResourceStatus 种质资源状态。
type ResourceStatus string

const (
	ResourceActive   ResourceStatus = "ACTIVE"   // 在库可用
	ResourceArchived ResourceStatus = "ARCHIVED" // 已归档，不再接受新批次
)

// Resource 为一份农业种质资源（物种级别的登记单元）。
type Resource struct {
	ID        string         `json:"id"`
	Code      string         `json:"code"` // 业务编码，全局唯一
	Name      string         `json:"name"`
	Species   string         `json:"species"`
	Category  string         `json:"category"` // 如 粮食作物/蔬菜/果树
	Status    ResourceStatus `json:"status"`
	Remark    string         `json:"remark,omitempty"`
	Version   int64          `json:"version"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// AccessionStatus 种质 accession 状态。
type AccessionStatus string

const (
	AccessionRegistered AccessionStatus = "REGISTERED" // 已登记
	AccessionInStock    AccessionStatus = "IN_STOCK"   // 已有样本入库
	AccessionDepleted   AccessionStatus = "DEPLETED"   // 库存耗尽
)

// Accession 为资源下的一份种质样本来源登记（种质编号）。
type Accession struct {
	ID          string          `json:"id"`
	ResourceID  string          `json:"resource_id"`
	AccessionNo string          `json:"accession_no"` // 种质编号，全局唯一
	Origin      string          `json:"origin"`
	Donor       string          `json:"donor"`
	CollectedAt string          `json:"collected_at,omitempty"`
	Status      AccessionStatus `json:"status"`
	Version     int64           `json:"version"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// BatchKind 批次类型。
type BatchKind string

const (
	BatchOriginal     BatchKind = "ORIGINAL"     // 原始批次（登记时建立）
	BatchRegeneration BatchKind = "REGENERATION" // 繁育批次
	BatchRestock      BatchKind = "RESTOCK"      // 回存批次
)

// BatchStatus 批次状态。
type BatchStatus string

const (
	BatchActive    BatchStatus = "ACTIVE"    // 在库
	BatchExhausted BatchStatus = "EXHAUSTED" // 数量耗尽
	BatchClosed    BatchStatus = "CLOSED"    // 已关闭（回存验收后母批关闭）
	BatchDestroyed BatchStatus = "DESTROYED" // 已全部销毁
)

// Batch 为同一 accession 下的一批种子，数量守恒的单位为粒。
// 守恒约束：QtyTotal = QtyAvailable + QtyFrozen + QtyOutbound + QtyDestroyed。
type Batch struct {
	ID            string      `json:"id"`
	AccessionID   string      `json:"accession_id"`
	BatchNo       string      `json:"batch_no"`
	Kind          BatchKind   `json:"kind"`
	MotherBatchID string      `json:"mother_batch_id,omitempty"` // 回存/繁育批次的母批
	Unit          string      `json:"unit"`                      // 计量单位，如 粒/克
	QtyTotal      int64       `json:"qty_total"`
	QtyAvailable  int64       `json:"qty_available"`
	QtyFrozen     int64       `json:"qty_frozen"`   // 被出库申请冻结
	QtyOutbound   int64       `json:"qty_outbound"` // 已出库
	QtyDestroyed  int64       `json:"qty_destroyed"`
	Status        BatchStatus `json:"status"`
	Version       int64       `json:"version"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	ClosedAt      *time.Time  `json:"closed_at,omitempty"`
}

// CheckConservation 校验批次数量守恒，返回偏差（0 表示守恒）。
func (b *Batch) CheckConservation() int64 {
	return b.QtyTotal - (b.QtyAvailable + b.QtyFrozen + b.QtyOutbound + b.QtyDestroyed)
}
