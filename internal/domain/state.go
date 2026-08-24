package domain

import "germplasm/internal/apperr"

// transitions 定义各实体的合法状态转换表，详见 docs/02-states.md。
var transitions = map[string]map[string][]string{
	"outbound": {
		string(OutboundPending):   {string(OutboundApproved), string(OutboundRejected), string(OutboundCancelled)},
		string(OutboundApproved):  {string(OutboundFulfilled), string(OutboundCancelled)},
		string(OutboundRejected):  {},
		string(OutboundFulfilled): {},
		string(OutboundCancelled): {},
	},
	"plan": {
		// COMPLETED 仅可由回存验收从 ACTIVE 推进；TIMEOUT 为繁育超时终态，
		// 拒绝新的回存链路（只能 CLOSED），见 docs/02-states.md。
		string(PlanActive):    {string(PlanCompleted), string(PlanTimeout), string(PlanClosed)},
		string(PlanTimeout):   {string(PlanClosed)},
		string(PlanCompleted): {string(PlanClosed)},
		string(PlanClosed):    {},
	},
	"restock": {
		string(RestockPending):  {string(RestockAccepted), string(RestockRejected)},
		string(RestockAccepted): {},
		string(RestockRejected): {},
	},
	"batch": {
		string(BatchActive):    {string(BatchExhausted), string(BatchClosed), string(BatchDestroyed)},
		string(BatchExhausted): {string(BatchClosed), string(BatchDestroyed)},
		string(BatchClosed):    {},
		string(BatchDestroyed): {},
	},
	"rule": {
		string(RuleDraft):   {string(RuleActive)},
		string(RuleActive):  {string(RuleRetired)},
		string(RuleRetired): {},
	},
	"destruction": {
		string(DestructionPending):  {string(DestructionApproved), string(DestructionRejected)},
		string(DestructionApproved): {},
		string(DestructionRejected): {},
	},
}

// CanTransition 判断实体能否从 from 转换到 to。
func CanTransition(entity, from, to string) bool {
	for _, t := range transitions[entity][from] {
		if t == to {
			return true
		}
	}
	return false
}

// MustTransition 校验状态转换，非法时返回状态冲突错误。
func MustTransition(entity, from, to string) error {
	if !CanTransition(entity, from, to) {
		return apperr.Statef("%s 不允许从 %s 转换到 %s", entity, from, to)
	}
	return nil
}

// TerminalOutbound 判断出库申请是否处于终态。
func TerminalOutbound(s OutboundStatus) bool {
	return s == OutboundRejected || s == OutboundFulfilled || s == OutboundCancelled
}
