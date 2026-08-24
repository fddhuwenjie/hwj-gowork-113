package domain

import "germplasm/internal/apperr"

// FreezePlanItem 为待冻结的样本拆分计划项。
type FreezePlanItem struct {
	Sample   Sample // 原样本
	TakeQty  int64  // 从原样本中冻结的数量
	Location string // 样本所在库位
}

// PlanFreeze 在在库样本上按先进先出（创建时间升序）规划冻结数量。
// 样本数量必须守恒：冻结总量不得超过批次可用量，也不得超过样本在库量之和。
// 返回每个样本需要冻结的数量；不足时返回数量守恒错误。
func PlanFreeze(samples []Sample, qty int64) ([]FreezePlanItem, error) {
	if qty <= 0 {
		return nil, apperr.Validation("冻结数量必须为正数")
	}
	var available int64
	for _, s := range samples {
		if s.Status == SampleInStock {
			available += s.Qty
		}
	}
	if available < qty {
		return nil, apperr.Quantityf("库存不足：需要 %d，可用 %d", qty, available)
	}
	remaining := qty
	var plan []FreezePlanItem
	for _, s := range samples {
		if s.Status != SampleInStock || remaining == 0 {
			continue
		}
		take := s.Qty
		if take > remaining {
			take = remaining
		}
		plan = append(plan, FreezePlanItem{Sample: s, TakeQty: take, Location: s.LocationID})
		remaining -= take
	}
	if remaining != 0 {
		return nil, apperr.Quantityf("库存不足：差额 %d", remaining)
	}
	return plan, nil
}

// ValidateRatio 校验 0~1 区间的比例字段。
func ValidateRatio(name string, v float64) error {
	if v < 0 || v > 1 {
		return apperr.Validationf("%s 必须在 0~1 之间: %v", name, v)
	}
	return nil
}
