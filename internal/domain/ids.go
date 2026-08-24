// Package domain 定义领域模型、状态机与核心业务规则。
package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

// idCounter 为进程内单调计数器，保证同一时间戳下 ID 仍可排序。
var idCounter atomic.Uint64

// NewID 生成带前缀的全局唯一 ID，形如 "bat_018f3a2c1b00042a9f2c"。
// ID 由时间戳 + 单调计数器 + 随机数组成，字典序与创建顺序一致，
// 从而保证 (created_at, id) 复合排序下稳定分页不重不漏。
func NewID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("生成随机 ID 失败: %w", err))
	}
	nano := time.Now().UnixNano()
	seq := idCounter.Add(1)
	return fmt.Sprintf("%s_%012x%06x%s", prefix, nano, seq&0xFFFFFF, hex.EncodeToString(b[:4]))
}

// 实体 ID 前缀，便于日志与排障时识别实体类型。
const (
	PrefixResource    = "res"
	PrefixAccession   = "acc"
	PrefixBatch       = "bat"
	PrefixSample      = "sam"
	PrefixLocation    = "loc"
	PrefixSensor      = "sen"
	PrefixRule        = "rul"
	PrefixOutbound    = "out"
	PrefixFreeze      = "frz"
	PrefixPlan        = "pln"
	PrefixObservation = "obs"
	PrefixPurityTest  = "pts"
	PrefixRestock     = "rst"
	PrefixDestruction = "dst"
	PrefixLineage     = "lin"
	PrefixJob         = "job"
	PrefixAlert       = "alt"
)
