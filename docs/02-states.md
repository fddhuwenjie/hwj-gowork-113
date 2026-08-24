# 02 状态转换表

本文档列出各实体的合法状态转换。状态机定义于 `internal/domain/state.go`（`transitions` 表，非法转换返回 `STATE_CONFLICT`）；样本、告警与作业的状态随业务动作联动变更，未列入 `transitions` 表，在对应服务/仓储中维护。

## 出库申请（outbound_requests.status）

| 当前状态 | 目标状态 | 触发动作 | 附带副作用 |
| --- | --- | --- | --- |
| PENDING | APPROVED | `POST /api/v1/outbound-requests/{id}/approve`（乐观锁） | 同一事务内：校验规则版本为 ACTIVE；FIFO 冻结在库样本（必要时拆分样本为 FROZEN）；写入冻结明细（ACTIVE）；批次 `qty_available −= qty`、`qty_frozen += qty`；校验出库前环境窗口；冻结规则版本与繁育目标；写历史快照与审计 |
| PENDING | REJECTED | `POST /api/v1/outbound-requests/{id}/reject`（乐观锁） | 仅状态变更 + 审计（终态） |
| PENDING | CANCELLED | `POST /api/v1/outbound-requests/{id}/cancel`（乐观锁） | 仅状态变更 + 审计（终态） |
| APPROVED | FULFILLED | `POST /api/v1/outbound-requests/{id}/fulfill`（乐观锁） | 同一事务内：校验审批后持续监控窗口；冻结样本转 OUTBOUND、冻结明细转 CONSUMED；批次 `qty_frozen −= qty`、`qty_outbound += qty`（可用与冻结均为 0 时批次转 EXHAUSTED）；写历史快照与审计（终态） |
| APPROVED | CANCELLED | `POST /api/v1/outbound-requests/{id}/cancel`（乐观锁） | 同一事务内释放全部冻结：样本回 IN_STOCK、明细转 RELEASED、批次 `qty_frozen −= qty`、`qty_available += qty`；审计（终态） |
| REJECTED / FULFILLED / CANCELLED | — | 终态，不允许任何转换 | — |

## 繁育计划（breeding_plans.status）

| 当前状态 | 目标状态 | 触发动作 | 附带副作用 |
| --- | --- | --- | --- |
| ACTIVE | COMPLETED | 回存验收通过（`POST /api/v1/restock-batches/{id}/accept`） | 在验收事务内联动转换，与创建新批次、谱系边、母批关闭、快照一起提交 |
| ACTIVE | TIMEOUT | 后台作业 `breeding_timeout_scan`（超过繁育期限） | 状态标记为 TIMEOUT 并产生 `BREEDING_TIMEOUT` 告警（去重） |
| ACTIVE | CLOSED | `POST /api/v1/breeding-plans/{id}/close`（乐观锁） | 状态变更 + 历史快照 + 审计（终态） |
| TIMEOUT | CLOSED | 同上（close） | 同上 |
| COMPLETED | CLOSED | 同上（close） | 同上 |
| CLOSED | — | 终态，不允许任何转换 | — |

## 回存验收单（restock_batches.status）

| 当前状态 | 目标状态 | 触发动作 | 附带副作用 |
| --- | --- | --- | --- |
| PENDING | ACCEPTED | `POST /api/v1/restock-batches/{id}/accept`（乐观锁） | 复核封存合格结论（覆盖率/纯度须满足冻结规则门槛，否则 QUALITY_VIOLATION）；创建 RESTOCK 新批次（qty 全部可用）并回填 `new_batch_id`；建立母批→子批谱系边（RESTOCK）；耗尽母批关闭（CLOSED）；繁育计划转 COMPLETED；写历史快照与审计（终态） |
| PENDING | REJECTED | `POST /api/v1/restock-batches/{id}/reject`（乐观锁，必填驳回原因） | 回填 `reject_reason`；写历史快照与审计（终态） |
| ACCEPTED / REJECTED | — | 终态，不允许任何转换 | — |

## 批次（batches.status）

| 当前状态 | 目标状态 | 触发动作 | 附带副作用 |
| --- | --- | --- | --- |
| ACTIVE | EXHAUSTED | 出库执行（fulfill）后可用量与冻结量均为 0 | 随出库事务联动转换 |
| ACTIVE | CLOSED | 回存验收后母批可用量与冻结量均为 0 | 随验收事务联动转换，记录 `closed_at`（终态） |
| ACTIVE | DESTROYED | 销毁审批批准后可用、冻结、出库量均为 0 | 批次数量扣减、样本按 FIFO 销毁、库位释放，同一事务（终态） |
| EXHAUSTED | CLOSED | 回存验收后母批关闭 | 同上 |
| EXHAUSTED | DESTROYED | 销毁审批批准（全部数量已出库/销毁场景） | 同上 |
| CLOSED / DESTROYED | — | 终态，不允许任何转换 | — |

## 保存规则版本（rule_versions.status）

| 当前状态 | 目标状态 | 触发动作 | 附带副作用 |
| --- | --- | --- | --- |
| DRAFT | ACTIVE | `POST /api/v1/rules/{id}/activate` | 同一事务内同 code 的旧 ACTIVE 版本退役为 RETIRED，记录 `effective_from`；审计 |
| ACTIVE | RETIRED | 同 code 新版本启用时联动 | 随新版本 activate 事务联动转换（终态） |
| RETIRED | — | 终态，不允许任何转换 | — |

## 销毁审批（destruction_approvals.status）

| 当前状态 | 目标状态 | 触发动作 | 附带副作用 |
| --- | --- | --- | --- |
| PENDING | APPROVED | `POST /api/v1/destruction-approvals/{id}/approve`（乐观锁） | 同一事务内：批次 `qty_available −= qty`、`qty_destroyed += qty`；按 FIFO 销毁在库样本（整样本转 DESTROYED 或拆分出 `-D` 后缀销毁样本）；释放被销毁样本的库位占用；全部数量耗尽时批次转 DESTROYED；记录审批人 `approver`；审计（终态） |
| PENDING | REJECTED | `POST /api/v1/destruction-approvals/{id}/reject`（乐观锁） | 记录审批人 `approver`；审计（终态） |
| APPROVED / REJECTED | — | 终态，不允许任何转换 | — |

## 样本（samples.status）

样本状态随出库/销毁/释放等业务动作联动变更（不在 `transitions` 表中）：

| 当前状态 | 目标状态 | 触发动作 | 附带副作用 |
| --- | --- | --- | --- |
| IN_STOCK | FROZEN | 出库审批（approve） | 整样本冻结或拆分：原样本保留剩余数量（IN_STOCK），新 `-F` 后缀样本承载冻结数量（FROZEN）；写入冻结明细；批次数量同步 |
| FROZEN | OUTBOUND | 出库执行（fulfill） | 冻结明细转 CONSUMED；批次 `qty_frozen` 转 `qty_outbound` |
| FROZEN | IN_STOCK | 取消已审批出库申请（cancel）释放冻结 | 冻结明细转 RELEASED；批次 `qty_frozen` 转回 `qty_available` |
| IN_STOCK | DESTROYED | 销毁审批批准（approve） | 整样本销毁或拆分出 `-D` 后缀销毁样本；库位占用释放；批次数量同步 |
| OUTBOUND / DESTROYED | — | 终态 | — |

## 告警（alerts.status）

| 当前状态 | 目标状态 | 触发动作 | 附带副作用 |
| --- | --- | --- | --- |
| OPEN | ACKED | `POST /api/v1/alerts/{id}/ack` | 记录 `acked_at` |
| ACKED | — | 终态 | — |

## 后台作业（jobs.status）

| 当前状态 | 目标状态 | 触发动作 | 附带副作用 |
| --- | --- | --- | --- |
| PENDING | RUNNING | 调度器抢占到期作业（`next_run_at <= now`） | `attempts` 递增 |
| RUNNING | DONE | 作业执行成功 | 标记完成 |
| RUNNING | PENDING | 作业执行失败且未超过 `max_attempts` | 记录 `last_error`，按指数退避（`2^(attempts-1)` 秒）重排 `next_run_at` |
| RUNNING | FAILED | 作业执行失败且已达 `max_attempts` | 记录 `last_error`（终态） |
| RUNNING | PENDING | 进程重启恢复（启动时 `RecoverStuck`） | 崩溃遗留的 RUNNING 作业全部重置为 PENDING，等待重新执行 |
| DONE / FAILED | — | 终态 | — |

## 其他实体的状态说明

- **资源（resources.status）**：`ACTIVE` → `ARCHIVED`（`POST /api/v1/resources/{id}/archive`，乐观锁）；归档后不接受新 accession 登记。
- **Accession（accessions.status）**：`REGISTERED` → `IN_STOCK`（建立首个原始批次时联动）；另有 `DEPLETED`（库存耗尽）取值。
- **库位（locations.status）**：创建为 `IDLE`；占用满容时转 `ACTIVE`；`DISABLED` 为停用（不可再占用）。
- **冻结明细（outbound_freezes.status）**：`ACTIVE` → `RELEASED`（取消释放）/ `CONSUMED`（出库消耗）。
- **纯度检测结论（purity_tests.verdict）**：`PENDING` → `PASS` / `FAIL`（封存时按规则计算，`sealed=1` 后只读）。
- **传感器（sensors.status）**：`ONLINE` / `OFFLINE`（注册时为 ONLINE）。
