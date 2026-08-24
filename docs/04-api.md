# 04 接口契约

本文档按 `internal/httpx/router.go` 的真实路由逐一说明。所有接口均为 HTTP JSON，响应 `Content-Type: application/json; charset=utf-8`。请求体解析启用 `DisallowUnknownFields`（未知字段报 `VALIDATION`），空请求体视为 `{}`。

## 通用约定

### 请求头

| 头 | 说明 |
| --- | --- |
| `Content-Type: application/json` | 写接口请求体为 JSON |
| `X-Actor` | 操作者标识，写入审计日志；缺省为 `system` |

### 统一错误格式

所有错误响应统一为：

```json
{
  "error": {
    "code": "STATE_CONFLICT",
    "message": "outbound 不允许从 FULFILLED 转换到 APPROVED",
    "details": {}
  }
}
```

`details` 可选，如 `ENV_WINDOW_VIOLATION` 会附带环境窗口评估报告。

### 错误码表

| 错误码 | HTTP 状态 | 含义 |
| --- | --- | --- |
| VALIDATION | 400 | 请求参数校验失败（必填缺失、时间格式非法、比例超界、JSON 非法等） |
| NOT_FOUND | 404 | 资源不存在 |
| CONFLICT | 409 | 唯一性冲突（如编码/编号重复、库位容量不足） |
| STATE_CONFLICT | 409 | 状态机不允许的转换（含已封存重复封存等） |
| OPTIMISTIC_LOCK | 409 | 乐观锁版本不匹配，需刷新后重试 |
| IDEMPOTENCY_CONFLICT | 409 | 幂等键已用于不同的请求体 |
| QUANTITY_VIOLATION | 409 | 数量守恒被破坏或库存不足 |
| QUALITY_VIOLATION | 409 | 检测覆盖不足或纯度不合格（不得回存为合格批次） |
| ENV_WINDOW_VIOLATION | 409 | 环境时间窗覆盖不足或存在越限读数 |
| INTERNAL | 500 | 内部错误（含未捕获 panic） |

### 分页

列表接口使用稳定游标分页：

- 查询参数：`cursor`（上一页返回的 `next_cursor`）、`limit`（默认 20，最大 200）；
- 排序：按 `(created_at, id)` 升序，游标为 Base64URL 编码的 `created_at|id`；
- 响应：`{"items": [...], "next_cursor": "..."}`，无下一页时省略 `next_cursor`；
- 例外：`GET /api/v1/audit-logs` 以自增 ID 为游标（`cursor` 传上一页返回的 `next_cursor` 数字字符串）。

### 乐观锁

状态变更类接口（approve/reject/fulfill/cancel/close/seal/accept/ack 等，以及资源归档、样本分配库位）请求体须携带实体当前 `version`：

```json
{"version": 1}
```

版本不匹配返回 `OPTIMISTIC_LOCK`。

### 幂等键

`POST /api/v1/outbound-requests`、`POST /api/v1/purity-tests`、`POST /api/v1/restock-batches` 支持请求体字段 `idempotency_key`：同键同请求体重放返回首个创建的实体（HTTP 201 与首个实体内容）；同键不同请求体返回 `IDEMPOTENCY_CONFLICT`。

### 时间格式

请求中的时间字段使用 RFC3339（如 `2026-08-24T12:00:00Z`），响应中的时间为 RFC3339Nano（UTC）。`recorded_at` / `observed_at` / `tested_at` 缺省时取当前时间。

---

## 健康检查

### GET /healthz

数据库可连通即健康，无需请求体。

成功 `200`：

```json
{"status": "ok", "time": "2026-08-24T12:00:00Z"}
```

数据库不可达时 `503`：`{"status": "unhealthy", "db": "<错误>"}`。

## 资源与 accession

### POST /api/v1/resources

登记种质资源。请求体：

```json
{"code": "RES001", "name": "水稻种质", "species": "Oryza sativa", "category": "粮食作物", "remark": ""}
```

- 校验：`code`、`name`、`species` 必填。
- 成功 `201`：资源对象（`status` 为 `ACTIVE`，`version` 为 1）。
- 错误：`VALIDATION`（必填缺失）、`CONFLICT`（code 重复）。

### GET /api/v1/resources

分页查询资源。参数：`cursor`、`limit`。成功 `200`：`{"items": [...], "next_cursor": "..."}`。

### GET /api/v1/resources/{id}

查询资源详情。成功 `200`：资源对象。错误：`NOT_FOUND`。

### POST /api/v1/resources/{id}/archive

归档资源（乐观锁）。请求体：`{"version": 1}`。成功 `200`：`status` 为 `ARCHIVED` 的资源对象。错误：`NOT_FOUND`、`OPTIMISTIC_LOCK`。

### POST /api/v1/accessions

在资源下登记 accession。请求体：

```json
{"resource_id": "res_...", "accession_no": "ACC001", "origin": "云南", "donor": "某农科院", "collected_at": "2025-05-01"}
```

- 校验：`resource_id`、`accession_no` 必填；资源必须存在且为 `ACTIVE`。
- 成功 `201`：accession 对象（`status` 为 `REGISTERED`）。
- 错误：`VALIDATION`、`NOT_FOUND`（资源不存在）、`STATE_CONFLICT`（资源已归档）、`CONFLICT`（accession_no 重复）。

### GET /api/v1/accessions

分页查询 accession。参数：`resource_id`（可选过滤）、`cursor`、`limit`。成功 `200`：分页对象。

### GET /api/v1/accessions/{id}

查询 accession 详情。成功 `200`。错误：`NOT_FOUND`。

## 批次、分装、样本与库位

### POST /api/v1/batches

建立原始批次（`ORIGINAL`，数量全部可用）。请求体：

```json
{"accession_id": "acc_...", "batch_no": "B2026-001", "unit": "粒", "qty_total": 10000}
```

- 校验：`accession_id`、`batch_no` 必填，`qty_total` 为正数；`unit` 缺省为 `粒`。
- 副作用：所属 accession 由 `REGISTERED` 转 `IN_STOCK`。
- 成功 `201`：批次对象。
- 错误：`VALIDATION`、`NOT_FOUND`（accession 不存在）、`CONFLICT`（batch_no 重复）。

### GET /api/v1/batches

分页查询批次。参数：`accession_id`、`status`（可选过滤）、`cursor`、`limit`。

### GET /api/v1/batches/{id}

查询批次详情。错误：`NOT_FOUND`。

### POST /api/v1/batches/{id}/split

样本分装。请求体：

```json
{"batch_id": "", "quantities": [3000, 3000, 4000]}
```

（`batch_id` 以路径 `{id}` 为准，请求体中留空即可；`quantities` 每个元素为正数，总和不得超过批次未分装可用量。）

- 成功 `201`：`{"items": [样本对象...]}`（样本编号形如 `B2026-001-S0001`）。
- 错误：`VALIDATION`（列表为空/数量非正）、`STATE_CONFLICT`（批次非 ACTIVE）、`QUANTITY_VIOLATION`（分装总量超未分装可用量）。

### GET /api/v1/samples

分页查询样本。参数：`batch_id`、`status`（可选过滤）、`cursor`、`limit`。

### GET /api/v1/samples/{id}

查询样本详情。错误：`NOT_FOUND`。

### POST /api/v1/samples/{id}/assign-location

样本分配库位（乐观锁）。请求体：

```json
{"location_id": "loc_...", "version": 1}
```

- 校验：样本为 `IN_STOCK` 且未分配库位；库位存在、未停用且有剩余容量。
- 副作用：库位 `occupied + 1`（占满转 `ACTIVE`）。
- 成功 `200`：样本对象（含 `location_id`）。
- 错误：`NOT_FOUND`（样本/库位不存在）、`OPTIMISTIC_LOCK`、`STATE_CONFLICT`（样本状态不允许或已分配）、`CONFLICT`（库位容量不足或已停用）。

### POST /api/v1/locations

创建低温库位。请求体：

```json
{"code": "C01-R02-B03-S04", "chamber": "C01", "rack": "R02", "box": "B03", "slot": "S04", "capacity": 10}
```

- 校验：`code`、`chamber` 必填，`capacity` 为正数。
- 成功 `201`：库位对象（`status` 为 `IDLE`）。
- 错误：`VALIDATION`、`CONFLICT`（code 重复）。

### GET /api/v1/locations

分页查询库位。参数：`chamber`（可选过滤）、`cursor`、`limit`。

## 环境监测与保存规则

### POST /api/v1/sensors

注册环境传感器。请求体：

```json
{"code": "SEN-C01-T", "chamber": "C01", "metric": "TEMPERATURE"}
```

- 校验：`code`、`chamber` 必填；`metric` 为 `TEMPERATURE` 或 `HUMIDITY`。
- 成功 `201`：传感器对象（`status` 为 `ONLINE`）。
- 错误：`VALIDATION`、`CONFLICT`（code 重复）。

### GET /api/v1/sensors

分页查询传感器。参数：`chamber`（可选过滤）、`cursor`、`limit`。

### POST /api/v1/sensors/{id}/readings

写入环境读数（只增不改）。请求体：

```json
{"value": -18.5, "recorded_at": "2026-08-24T12:00:00Z"}
```

`recorded_at` 可省略（取当前时间）。成功 `201`：读数对象（`metric` 继承传感器）。错误：`VALIDATION`（时间非法）、`NOT_FOUND`（传感器不存在）。

### GET /api/v1/readings

分页查询环境读数。参数：`sensor_id`（可选过滤）、`cursor`、`limit`。

### POST /api/v1/rules

创建保存规则版本（草稿，版本号自动递增）。请求体：

```json
{
  "code": "RULE-RICE",
  "min_temp": -20, "max_temp": -15,
  "min_humidity": 30, "max_humidity": 50,
  "window_before_hours": 72, "window_after_hours": 24,
  "min_coverage": 0.9, "min_purity": 0.98
}
```

- 校验：`code` 必填；`min_temp < max_temp`、`min_humidity < max_humidity`；`min_coverage`、`min_purity` 在 0~1；窗口小时数非负。
- 成功 `201`：规则版本对象（`status` 为 `DRAFT`）。
- 错误：`VALIDATION`。

### GET /api/v1/rules

分页查询规则版本。参数：`code`（可选过滤）、`cursor`、`limit`。

### POST /api/v1/rules/{id}/activate

启用规则版本：同 code 旧 ACTIVE 版本退役，本版本转 ACTIVE 并记录 `effective_from`。无请求体。成功 `200`：规则版本对象。错误：`NOT_FOUND`、`STATE_CONFLICT`。

## 出库申请

### POST /api/v1/outbound-requests

创建出库申请（`PENDING`，支持幂等键）。请求体：

```json
{
  "request_no": "OUT-2026-001",
  "accession_id": "acc_...",
  "batch_id": "bat_...",
  "qty": 500,
  "purpose": "繁育扩繁",
  "breeding_target": "回存 5000 粒",
  "rule_version_id": "rul_...",
  "deadline": "2026-09-01T00:00:00Z",
  "idempotency_key": "req-001"
}
```

- 校验：`request_no`、`batch_id`、`rule_version_id` 必填；`qty` 为正数；`deadline` 为 RFC3339；批次属于指定 accession、状态为 ACTIVE 且可用量充足；规则版本存在。
- 成功 `201`：出库申请对象。
- 错误：`VALIDATION`、`NOT_FOUND`、`STATE_CONFLICT`（批次状态不允许）、`QUANTITY_VIOLATION`（可用量不足）、`CONFLICT`（request_no 重复）、`IDEMPOTENCY_CONFLICT`。

### GET /api/v1/outbound-requests

分页查询出库申请。参数：`status`（可选过滤）、`cursor`、`limit`。

### GET /api/v1/outbound-requests/{id}

查询出库申请详情。错误：`NOT_FOUND`。

### GET /api/v1/outbound-requests/{id}/freezes

查询申请的冻结明细。成功 `200`：`{"items": [冻结明细...]}`（含样本、库位、数量、状态）。错误：`NOT_FOUND`。

### POST /api/v1/outbound-requests/{id}/approve

审批出库申请（乐观锁，PENDING → APPROVED）。请求体：`{"version": 1}`。

- 副作用：冻结样本/库位/规则/繁育目标，写冻结明细与批次数量，写历史快照。
- 成功 `200`：申请对象（`status` 为 `APPROVED`）。
- 错误：`NOT_FOUND`、`OPTIMISTIC_LOCK`、`STATE_CONFLICT`（状态不允许/规则未启用）、`QUANTITY_VIOLATION`、`VALIDATION`（样本未分配库位）、`ENV_WINDOW_VIOLATION`（出库前窗口覆盖不足，`details` 含窗口评估报告）。

### POST /api/v1/outbound-requests/{id}/reject

驳回待审批申请（乐观锁，PENDING → REJECTED）。请求体：`{"version": 1}`。成功 `200`。错误：`NOT_FOUND`、`OPTIMISTIC_LOCK`、`STATE_CONFLICT`。

### POST /api/v1/outbound-requests/{id}/fulfill

执行出库（乐观锁，APPROVED → FULFILLED）。请求体：`{"version": 1}`。

- 副作用：冻结样本转 OUTBOUND、明细转 CONSUMED、批次冻结量转出库量（耗尽批次转 EXHAUSTED），写历史快照。
- 错误：`NOT_FOUND`、`OPTIMISTIC_LOCK`、`STATE_CONFLICT`（含无有效冻结明细）、`ENV_WINDOW_VIOLATION`（审批后持续监控窗口覆盖不足）。

### POST /api/v1/outbound-requests/{id}/cancel

取消申请（乐观锁，PENDING/APPROVED → CANCELLED）。请求体：`{"version": 1}`。

- 副作用：已审批申请取消时释放全部冻结（样本回 IN_STOCK、明细转 RELEASED、批次数量回补）。
- 错误：`NOT_FOUND`、`OPTIMISTIC_LOCK`、`STATE_CONFLICT`。

## 繁育计划、田间观察与纯度检测

### POST /api/v1/breeding-plans

基于已出库申请建立繁育计划（一个申请只能建一个计划）。请求体：

```json
{
  "plan_no": "PLAN-2026-001",
  "outbound_request_id": "out_...",
  "target_qty": 5000,
  "plot": "试验田 A-3",
  "deadline": "2026-12-31T00:00:00Z"
}
```

- 校验：`plan_no`、`outbound_request_id` 必填；`target_qty` 为正数；`deadline` 为 RFC3339；出库申请必须为 `FULFILLED`。
- 成功 `201`：计划对象（`status` 为 `ACTIVE`，`batch_id` 取申请批次），写历史快照。
- 错误：`VALIDATION`、`NOT_FOUND`、`STATE_CONFLICT`（申请未出库）、`CONFLICT`（plan_no 重复/申请已建计划）。

### GET /api/v1/breeding-plans

分页查询繁育计划。参数：`status`（可选过滤）、`cursor`、`limit`。

### GET /api/v1/breeding-plans/{id}

查询繁育计划详情。错误：`NOT_FOUND`。

### POST /api/v1/breeding-plans/{id}/close

关闭繁育计划（乐观锁，ACTIVE/COMPLETED/TIMEOUT → CLOSED）。请求体：`{"version": 1}`。写历史快照。错误：`NOT_FOUND`、`OPTIMISTIC_LOCK`、`STATE_CONFLICT`。

### POST /api/v1/breeding-plans/{id}/observations

追加田间观察（仅 ACTIVE 计划）。请求体：

```json
{"observed_at": "2026-08-24T12:00:00Z", "germination_rate": 0.92, "vigor": "强", "notes": "出苗整齐"}
```

`observed_at` 可省略；`germination_rate` 须在 0~1。成功 `201`：观察记录对象。错误：`VALIDATION`、`NOT_FOUND`、`STATE_CONFLICT`。

### GET /api/v1/breeding-plans/{id}/observations

查询计划的田间观察记录。成功 `200`：`{"items": [...]}`。错误：`NOT_FOUND`。

### POST /api/v1/purity-tests

登记纯度检测（`verdict` 为 PENDING，支持幂等键）。请求体：

```json
{
  "plan_id": "pln_...",
  "sample_qty": 200,
  "coverage_ratio": 0.95,
  "purity_rate": 0.99,
  "tested_at": "2026-08-24T12:00:00Z",
  "idempotency_key": "test-001"
}
```

- 校验：`plan_id` 必填；`sample_qty` 为正数；`coverage_ratio`、`purity_rate` 在 0~1；`tested_at` 可省略；计划未关闭。
- 成功 `201`：检测对象。
- 错误：`VALIDATION`、`NOT_FOUND`、`STATE_CONFLICT`（计划已关闭）、`IDEMPOTENCY_CONFLICT`。

### GET /api/v1/purity-tests

分页查询纯度检测。参数：`plan_id`（可选过滤）、`cursor`、`limit`。

### GET /api/v1/purity-tests/{id}

查询检测详情。错误：`NOT_FOUND`。

### POST /api/v1/purity-tests/{id}/seal

封存质量判定（乐观锁）。请求体：`{"version": 1}`。

- 判定：按计划母批绑定的冻结规则计算 `verdict`（覆盖率或纯度低于门槛即 `FAIL`）；封存后 `sealed=true`、记录 `sealed_at`，结论只读；每个计划只允许一条封存结论。
- 成功 `200`：检测对象（含 `verdict`、`sealed`、`sealed_at`），写历史快照。
- 错误：`NOT_FOUND`、`OPTIMISTIC_LOCK`、`STATE_CONFLICT`（已封存/计划已有封存结论）。

## 回存验收与销毁审批

### POST /api/v1/restock-batches

创建回存验收单（`PENDING`，支持幂等键）。请求体：

```json
{"request_no": "RS-2026-001", "plan_id": "pln_...", "qty": 5000, "idempotency_key": "rs-001"}
```

- 校验：`request_no`、`plan_id` 必填；`qty` 为正数；计划必须为 `ACTIVE`。
- 成功 `201`：验收单对象。
- 错误：`VALIDATION`、`NOT_FOUND`、`STATE_CONFLICT`、`CONFLICT`（request_no 重复）、`IDEMPOTENCY_CONFLICT`。

### GET /api/v1/restock-batches

分页查询回存验收单。参数：`status`（可选过滤）、`cursor`、`limit`。

### GET /api/v1/restock-batches/{id}

查询验收单详情。错误：`NOT_FOUND`。

### POST /api/v1/restock-batches/{id}/accept

回存验收通过（乐观锁，PENDING → ACCEPTED）。请求体：`{"version": 1}`。

- 质量复核：必须存在封存结论且 `verdict` 为 PASS，覆盖率与纯度满足冻结规则门槛。
- 副作用：创建 RESTOCK 新批次（回填 `new_batch_id`）、建立母批→子批谱系边、耗尽母批关闭、计划转 COMPLETED、写历史快照。
- 错误：`NOT_FOUND`、`OPTIMISTIC_LOCK`、`STATE_CONFLICT`、`QUALITY_VIOLATION`（无封存结论或覆盖/纯度不达标）。

### POST /api/v1/restock-batches/{id}/reject

驳回回存验收单（乐观锁）。请求体：

```json
{"version": 1, "reason": "纯度不达标"}
```

`reason` 必填。写历史快照。错误：`VALIDATION`、`NOT_FOUND`、`OPTIMISTIC_LOCK`、`STATE_CONFLICT`。

### POST /api/v1/destruction-approvals

提交销毁申请（`PENDING`）。请求体：

```json
{"batch_id": "bat_...", "qty": 100, "reason": "霉变报废"}
```

- 校验：`batch_id` 必填；`qty` 为正数；`reason` 必填；批次非 CLOSED/DESTROYED 且可用量充足。
- 成功 `201`：审批单对象。
- 错误：`VALIDATION`、`NOT_FOUND`、`STATE_CONFLICT`、`QUANTITY_VIOLATION`。

### GET /api/v1/destruction-approvals

分页查询销毁审批单。参数：`batch_id`、`status`（可选过滤）、`cursor`、`limit`。

### POST /api/v1/destruction-approvals/{id}/approve

批准销毁（乐观锁，PENDING → APPROVED）。请求体：`{"version": 1}`。

- 副作用：批次数量扣减、样本按 FIFO 销毁（整样本或拆分）、库位释放、批次全毁时转 DESTROYED、记录审批人。
- 错误：`NOT_FOUND`、`OPTIMISTIC_LOCK`、`STATE_CONFLICT`、`QUANTITY_VIOLATION`。

### POST /api/v1/destruction-approvals/{id}/reject

驳回销毁申请（乐观锁，PENDING → REJECTED）。请求体：`{"version": 1}`。记录审批人。错误：`NOT_FOUND`、`OPTIMISTIC_LOCK`、`STATE_CONFLICT`。

## 谱系、快照、告警、审计与作业

### GET /api/v1/lineage

查询批次谱系。参数：`batch_id`（必填）。成功 `200`：

```json
{"batch_id": "bat_...", "parents": [谱系边...], "children": [谱系边...]}
```

错误：`VALIDATION`（缺少 batch_id）、`NOT_FOUND`。

### GET /api/v1/snapshots

分页查询历史快照。参数：`entity_type`、`entity_id`（可选过滤）、`cursor`、`limit`。快照对象含 `event` 与 `payload`（事件时刻实体状态 JSON）。

### GET /api/v1/alerts

分页查询告警。参数：`status`、`type`（可选过滤）、`cursor`、`limit`。

### POST /api/v1/alerts/{id}/ack

确认告警（OPEN → ACKED，记录 `acked_at`）。无请求体。成功 `200`：`{"status": "ACKED"}`。错误：`NOT_FOUND`。

### GET /api/v1/audit-logs

分页查询审计日志。参数：`entity_type`、`entity_id`（可选过滤）、`cursor`（自增 ID 游标）、`limit`。成功 `200`：`{"items": [...], "next_cursor": "123"}`。

### GET /api/v1/jobs

分页查询后台作业。参数：`status`（可选过滤）、`cursor`、`limit`。

## 风险巡检

### GET /api/v1/risk/locations

巡检全部冷库最近 24 小时的越限读数与未处理告警。成功 `200`：

```json
{
  "items": [
    {
      "chamber": "C01",
      "out_of_range_count": 3,
      "open_alerts": 1,
      "latest_temp": {"id": 12, "sensor_id": "sen_...", "metric": "TEMPERATURE", "value": -10.5, "recorded_at": "...", "created_at": "..."},
      "latest_humidity": {"id": 9, "sensor_id": "sen_...", "metric": "HUMIDITY", "value": 62, "recorded_at": "...", "created_at": "..."}
    }
  ]
}
```

仅返回存在越限读数或未处理告警的冷库。

### GET /api/v1/risk/inventory-variance

巡检全部批次的数量守恒与账实差异。成功 `200`：`{"items": [...]}`，元素含 `batch_id`、`batch_no`、`qty_total`、`book_sum`（账面分项合计）、`sample_sum`（样本汇总）、`available_diff`、`conserved`。仅返回不守恒或可用量与在库样本量不一致的批次。

### GET /api/v1/risk/germination-decline

巡检连续 2 次及以上发芽率下降的繁育计划。成功 `200`：`{"items": [{"plan_id": "...", "plan_no": "...", "consecutive_drops": 2, "rates": [0.95, 0.9, 0.85]}]}`。

### GET /api/v1/risk/pending-restock

查询待回存验收批次（分页）。参数：`cursor`、`limit`。成功 `200`：回存验收单分页对象（`status` 为 PENDING）。

### GET /api/v1/risk/lineage-anomalies

巡检谱系异常。成功 `200`：`{"items": [...]}`，元素含 `type`（`CYCLE` 有向环 / `SELF_LOOP` 自环 / `ORPHAN` 回存/繁育批次缺少母批）、`batch_id`、`message`；无异常时 `items` 为空数组。
