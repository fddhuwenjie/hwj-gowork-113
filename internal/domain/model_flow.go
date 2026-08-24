package domain

import "time"

// OutboundStatus 出库申请状态。
type OutboundStatus string

const (
	OutboundPending   OutboundStatus = "PENDING"   // 待审批
	OutboundApproved  OutboundStatus = "APPROVED"  // 已审批（样本/库位/规则/繁育目标已冻结）
	OutboundRejected  OutboundStatus = "REJECTED"  // 已驳回
	OutboundFulfilled OutboundStatus = "FULFILLED" // 已出库
	OutboundCancelled OutboundStatus = "CANCELLED" // 已取消（释放冻结）
)

// OutboundRequest 为出库申请。审批通过时冻结样本数量、库位、
// 保存规则版本与繁育目标，此后不可更改。
type OutboundRequest struct {
	ID             string         `json:"id"`
	RequestNo      string         `json:"request_no"`
	AccessionID    string         `json:"accession_id"`
	BatchID        string         `json:"batch_id"`
	Qty            int64          `json:"qty"`
	Purpose        string         `json:"purpose"`
	BreedingTarget string         `json:"breeding_target"` // 繁育目标，审批后冻结
	RuleVersionID  string         `json:"rule_version_id"` // 保存规则，审批后冻结
	Deadline       time.Time      `json:"deadline"`        // 交付截止时间
	Status         OutboundStatus `json:"status"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Version        int64          `json:"version"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// FreezeStatus 冻结明细状态。
type FreezeStatus string

const (
	FreezeActive   FreezeStatus = "ACTIVE"   // 冻结中
	FreezeReleased FreezeStatus = "RELEASED" // 已释放
	FreezeConsumed FreezeStatus = "CONSUMED" // 已随出库消耗
)

// OutboundFreeze 为出库审批时冻结的样本与库位明细。
type OutboundFreeze struct {
	ID         string       `json:"id"`
	RequestID  string       `json:"request_id"`
	SampleID   string       `json:"sample_id"`
	LocationID string       `json:"location_id"`
	Qty        int64        `json:"qty"`
	Status     FreezeStatus `json:"status"`
	CreatedAt  time.Time    `json:"created_at"`
}

// PlanStatus 繁育计划状态。
type PlanStatus string

const (
	PlanActive    PlanStatus = "ACTIVE"    // 繁育中
	PlanCompleted PlanStatus = "COMPLETED" // 已完成（回存验收通过）
	PlanTimeout   PlanStatus = "TIMEOUT"   // 超过繁育期限
	PlanClosed    PlanStatus = "CLOSED"    // 已关闭
)

// BreedingPlan 为出库后建立的繁育计划/繁育批次。
type BreedingPlan struct {
	ID                string     `json:"id"`
	PlanNo            string     `json:"plan_no"`
	OutboundRequestID string     `json:"outbound_request_id"`
	BatchID           string     `json:"batch_id"` // 母批
	TargetQty         int64      `json:"target_qty"`
	Plot              string     `json:"plot"`     // 田块
	Deadline          time.Time  `json:"deadline"` // 繁育期限
	Status            PlanStatus `json:"status"`
	Version           int64      `json:"version"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// FieldObservation 为一次田间观察记录。
type FieldObservation struct {
	ID              string    `json:"id"`
	PlanID          string    `json:"plan_id"`
	ObservedAt      time.Time `json:"observed_at"`
	GerminationRate float64   `json:"germination_rate"` // 发芽率 0~1
	Vigor           string    `json:"vigor"`
	Notes           string    `json:"notes"`
	CreatedAt       time.Time `json:"created_at"`
}

// TestVerdict 纯度检测结论。
type TestVerdict string

const (
	VerdictPending TestVerdict = "PENDING" // 未判定
	VerdictPass    TestVerdict = "PASS"    // 合格
	VerdictFail    TestVerdict = "FAIL"    // 不合格
)

// PurityTest 为一次纯度检测。封存（Sealed）后结论只读；
// 早于封存时间的迟到检测不得覆盖已有质量结论。
type PurityTest struct {
	ID             string      `json:"id"`
	PlanID         string      `json:"plan_id"`
	SampleQty      int64       `json:"sample_qty"`
	CoverageRatio  float64     `json:"coverage_ratio"` // 检测覆盖率 0~1
	PurityRate     float64     `json:"purity_rate"`    // 纯度合格率 0~1
	Verdict        TestVerdict `json:"verdict"`
	Sealed         bool        `json:"sealed"`
	SealedAt       *time.Time  `json:"sealed_at,omitempty"`
	TestedAt       time.Time   `json:"tested_at"`
	IdempotencyKey string      `json:"idempotency_key,omitempty"`
	Version        int64       `json:"version"`
	CreatedAt      time.Time   `json:"created_at"`
}

// RestockStatus 回存批次状态。
type RestockStatus string

const (
	RestockPending  RestockStatus = "PENDING"  // 待验收
	RestockAccepted RestockStatus = "ACCEPTED" // 验收通过（已创建新批次）
	RestockRejected RestockStatus = "REJECTED" // 验收驳回
)

// RestockBatch 为繁育后的回存验收单。验收通过时创建新批次并保留母批只读。
type RestockBatch struct {
	ID             string        `json:"id"`
	RequestNo      string        `json:"request_no"`
	PlanID         string        `json:"plan_id"`
	Qty            int64         `json:"qty"`
	Status         RestockStatus `json:"status"`
	NewBatchID     string        `json:"new_batch_id,omitempty"`
	RejectReason   string        `json:"reject_reason,omitempty"`
	IdempotencyKey string        `json:"idempotency_key,omitempty"`
	Version        int64         `json:"version"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// DestructionStatus 销毁审批状态。
type DestructionStatus string

const (
	DestructionPending  DestructionStatus = "PENDING"
	DestructionApproved DestructionStatus = "APPROVED"
	DestructionRejected DestructionStatus = "REJECTED"
)

// DestructionApproval 为批次销毁审批单。
type DestructionApproval struct {
	ID        string            `json:"id"`
	BatchID   string            `json:"batch_id"`
	Qty       int64             `json:"qty"`
	Reason    string            `json:"reason"`
	Status    DestructionStatus `json:"status"`
	Approver  string            `json:"approver,omitempty"`
	Version   int64             `json:"version"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// LineageEdge 为批次间的谱系边（母批 -> 子批）。
type LineageEdge struct {
	ID            string    `json:"id"`
	ResourceID    string    `json:"resource_id"`
	ParentBatchID string    `json:"parent_batch_id"`
	ChildBatchID  string    `json:"child_batch_id"`
	Relation      string    `json:"relation"` // RESTOCK / REGENERATION
	CreatedAt     time.Time `json:"created_at"`
}

// Snapshot 为出库、繁育、回存等关键事件的历史快照。
type Snapshot struct {
	ID         int64     `json:"id"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	Event      string    `json:"event"`
	Payload    string    `json:"payload"` // JSON 序列化的事件时刻实体状态
	CreatedAt  time.Time `json:"created_at"`
}

// Alert 为系统产生的告警。
type Alert struct {
	ID        string     `json:"id"`
	Type      string     `json:"type"` // ENV_OUT_OF_RANGE / OUTBOUND_DUE_SOON / BREEDING_TIMEOUT / RESTOCK_PENDING
	RefType   string     `json:"ref_type"`
	RefID     string     `json:"ref_id"`
	Message   string     `json:"message"`
	Status    string     `json:"status"` // OPEN / ACKED
	CreatedAt time.Time  `json:"created_at"`
	AckedAt   *time.Time `json:"acked_at,omitempty"`
}

// Job 为持久化后台作业，支持失败重试与重启恢复。
type Job struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Payload     string    `json:"payload"`
	Status      string    `json:"status"` // PENDING / RUNNING / DONE / FAILED
	Attempts    int       `json:"attempts"`
	MaxAttempts int       `json:"max_attempts"`
	NextRunAt   time.Time `json:"next_run_at"`
	LastError   string    `json:"last_error,omitempty"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// 作业类型。
const (
	JobEnvAlertScan        = "env_alert_scan"        // 环境告警扫描
	JobOutboundDueScan     = "outbound_due_scan"     // 出库临期扫描
	JobBreedingTimeoutScan = "breeding_timeout_scan" // 繁育超时扫描
	JobRestockPendingScan  = "restock_pending_scan"  // 回存验收超期扫描
)

// 作业状态。
const (
	JobPending = "PENDING"
	JobRunning = "RUNNING"
	JobDone    = "DONE"
	JobFailed  = "FAILED"
)

// 告警类型。
const (
	AlertEnvOutOfRange   = "ENV_OUT_OF_RANGE"
	AlertOutboundDueSoon = "OUTBOUND_DUE_SOON"
	AlertBreedingTimeout = "BREEDING_TIMEOUT"
	AlertRestockPending  = "RESTOCK_PENDING"
)
