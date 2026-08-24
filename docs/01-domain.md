# 01 领域说明

本文档描述农业种质资源低温保存、出库繁育与回存验收服务的领域概念与业务链。领域模型定义于 `internal/domain/`，业务规则编排于 `internal/service/`。

## 核心概念

### 种质资源（Resource）

物种级别的登记单元，是系统的顶层实体。包含业务编码 `code`（全局唯一）、名称、物种、类别（如粮食作物/蔬菜/果树）与备注。状态为 `ACTIVE`（在库可用）或 `ARCHIVED`（已归档，不再接受新 accession 登记）。

### Accession（种质编号登记）

资源下的一份种质样本来源登记，包含全局唯一的种质编号 `accession_no`、产地 `origin`、供种者 `donor`、采集时间 `collected_at`。状态包括 `REGISTERED`（已登记）、`IN_STOCK`（已有样本入库，建立首个原始批次时自动转换）、`DEPLETED`（库存耗尽）。

### 批次（Batch）

同一 accession 下的一批种子，是**数量守恒**的基本单位，计量单位默认为"粒"。批次类型（`kind`）：

- `ORIGINAL`：原始批次，登记时建立，初始数量全部可用；
- `REGENERATION`：繁育批次；
- `RESTOCK`：回存批次（回存验收通过时创建）。

回存/繁育批次通过 `mother_batch_id` 记录母批。批次数量四分项满足守恒公式：

```
qty_total = qty_available + qty_frozen + qty_outbound + qty_destroyed
```

- `qty_available`：可用量（可分装、可出库、可销毁）；
- `qty_frozen`：被出库申请冻结的数量；
- `qty_outbound`：已出库数量；
- `qty_destroyed`：已销毁数量。

批次状态：`ACTIVE`（在库）→ `EXHAUSTED`（数量耗尽）/ `CLOSED`（回存验收后耗尽母批关闭）/ `DESTROYED`（全部销毁）。

### 样本（Sample）

批次分装后的最小库存单位，存放在具体库位。分装不产生也不消灭数量：分装总量不得超过"批次可用量 − 已在库样本量"。出库审批与销毁审批按 FIFO（创建时间升序）选择样本，数量不足整样本时拆分：原样本保留剩余数量，新样本（编号加 `-F` / `-D` 后缀）承载冻结/销毁数量。样本状态：`IN_STOCK`（在库）、`FROZEN`（被冻结）、`OUTBOUND`（已出库）、`DESTROYED`（已销毁）。

### 低温库位（Location）

低温库中的最小库位，按"冷库 chamber / 架 rack / 盒 box / 格 slot"四级定位，编码 `code` 全局唯一（如 `C01-R02-B03-S04`）。`capacity` 为可容纳样本数，`occupied` 为已占用样本数；分配库位时占用 +1（占满时状态转为 `ACTIVE`，`DISABLED` 库位不可占用），销毁已分配样本时释放占用。状态：`IDLE`、`ACTIVE`、`DISABLED`。

### 环境传感器与读数（Sensor / SensorReading）

传感器部署在某个冷库，度量类型 `metric` 为 `TEMPERATURE`（温度，摄氏度）或 `HUMIDITY`（相对湿度，百分比），状态 `ONLINE` / `OFFLINE`。读数只增不改，记录采样值与采样时间 `recorded_at`，用于环境时间窗覆盖评估与越限告警。

### 保存规则版本（RuleVersion）

保存规则的**不可变版本**，按 `code` 递增版本号（`version_no`）。内容包括：

- 温湿度阈值：`min_temp` / `max_temp` / `min_humidity` / `max_humidity`；
- 出库前监控窗口 `window_before_hours` 与出库后监控窗口 `window_after_hours`（小时）；
- 检测覆盖率门槛 `min_coverage` 与纯度合格率门槛 `min_purity`（均 0~1）。

状态流转 `DRAFT` → `ACTIVE` → `RETIRED`：启用新版本时同 `code` 旧 `ACTIVE` 版本自动退役；出库申请在创建时绑定规则版本，审批后该版本被冻结，不再随规则演进变化。

### 出库申请与冻结明细（OutboundRequest / OutboundFreeze）

出库申请指定 accession、批次、数量、用途、**繁育目标**、**保存规则版本**与交付截止时间 `deadline`。创建时为 `PENDING`；审批通过（`APPROVED`）时在同一事务内：

1. 校验规则版本必须处于 `ACTIVE`；
2. 按 FIFO 在在库样本上规划冻结数量（必要时拆分样本），样本转为 `FROZEN`；
3. 写入冻结明细（`OutboundFreeze`，记录样本、库位、数量，状态 `ACTIVE`）；
4. 批次数量 `qty_available −= qty`、`qty_frozen += qty`；
5. 校验样本所在全部冷库的出库前环境窗口覆盖；
6. 冻结规则版本与繁育目标（此后不可更改）。

取消（`CANCELLED`）已审批申请时释放全部冻结（样本回 `IN_STOCK`，明细转 `RELEASED`，批次数量回补）；执行出库（`FULFILLED`）时冻结样本转 `OUTBOUND`、明细转 `CONSUMED`、批次 `qty_frozen` 转 `qty_outbound`，并校验审批后持续监控窗口的环境覆盖。

### 繁育计划（BreedingPlan）

出库后建立的繁育批次/计划，绑定出库申请（一个申请只能建一个计划，`outbound_request_id` 唯一），记录母批、繁育目标数量 `target_qty`、田块 `plot` 与繁育期限 `deadline`。状态：`ACTIVE`（繁育中）→ `COMPLETED`（回存验收通过）/ `TIMEOUT`（超过繁育期限，由后台作业标记）→ `CLOSED`（已关闭，终态）。

### 田间观察（FieldObservation）

繁育期间追加的观察记录，包含观察时间、发芽率 `germination_rate`（0~1）、长势 `vigor` 与备注。仅 `ACTIVE` 状态的计划可追加。连续 2 次及以上发芽率下降的计划会被风险巡检识别。

### 纯度检测（PurityTest）

对繁育计划的一次采样检测，记录抽样数量、检测覆盖率 `coverage_ratio`、纯度合格率 `purity_rate` 与检测时间 `tested_at`。结论 `verdict` 初始为 `PENDING`，**封存（seal）**时按出库申请冻结的规则版本计算：覆盖率或纯度低于门槛即 `FAIL`，否则 `PASS`。封存后结论只读；每个计划最多一条封存结论；`tested_at` 早于既有封存时刻的检测属于**迟到检测**，仅作只读记录，永远不得参与质量判定。

### 回存批次验收（RestockBatch）

繁育后的回存验收单（`request_no` 唯一），指定繁育计划与回存数量。验收通过（`ACCEPTED`）时在同一事务内：

1. 复核必须存在**封存合格**结论，且覆盖率与纯度满足冻结规则门槛，否则报 `QUALITY_VIOLATION`；
2. 创建 `RESTOCK` 新批次（批次号 `回存单号-RS`，数量全部可用）；
3. 建立母批 → 新批次的谱系边（`RESTOCK`）；
4. 母批耗尽（可用量与冻结量均为 0）则关闭（`CLOSED`，记录 `closed_at`）；
5. 繁育计划转为 `COMPLETED`；
6. 写入历史快照。

驳回（`REJECTED`）必须给出原因。母批与旧检测在验收后保持只读。

### 销毁审批（DestructionApproval）

批次销毁审批单，指定批次、数量与原因。批准时在同一事务内扣减批次数量（`qty_available −= qty`、`qty_destroyed += qty`）、按 FIFO 销毁在库样本（整样本销毁或拆分销毁，释放其库位占用）；当可用、冻结、出库量均为 0 时批次转为 `DESTROYED`。`CLOSED`/`DESTROYED` 批次不得申请销毁。

### 资源谱系（LineageEdge）

批次间的有向谱系边（母批 → 子批），`relation` 为 `RESTOCK` / `REGENERATION`，`(parent_batch_id, child_batch_id, relation)` 唯一。支持查询任一批次的直接母批与子批；异常检测识别自环（`SELF_LOOP`）、有向环（`CYCLE`）与缺少母批的回存/繁育批次（`ORPHAN`）。

### 历史快照（Snapshot）

出库审批/出库、繁育计划建立/关闭、检测封存、回存验收/驳回等关键事件发生时，将事件时刻的相关实体状态序列化为 JSON 存入 `snapshots` 表（`entity_type` + `entity_id` + `event` + `payload`），用于追溯与审计。

### 幂等键（IdempotencyKey）

出库申请、纯度检测、回存验收单的创建接口支持幂等键：同一 `(key, endpoint)` 重复提交且请求体一致时，返回首个创建的实体（不重复写入）；请求体不一致时报 `IDEMPOTENCY_CONFLICT`。实体表上 `idempotency_key` 列有唯一约束，`idempotency_keys` 表记录请求哈希与响应。

### 审计日志（AuditLog）

所有业务写操作在同一事务内追加审计日志（操作者 `actor` 来自 `X-Actor` 头，缺省 `system`；动作 `action` 如 `outbound.approve`；实体类型与 ID；`detail` 为 JSON），审计与业务写入同生共死。

### 告警（Alert）

系统产生的告警，类型包括：

- `ENV_OUT_OF_RANGE`：传感器最新读数越限，或超过 2 小时未上报读数；
- `OUTBOUND_DUE_SOON`：已审批出库申请临近交付截止仍未出库；
- `BREEDING_TIMEOUT`：繁育计划超过繁育期限；
- `RESTOCK_PENDING`：回存验收单创建超阈值小时数仍未验收。

告警状态 `OPEN` → `ACKED`（确认），同一 `(type, ref_type, ref_id)` 已有 `OPEN` 告警时去重不重复插入。

### 后台作业（Job）

持久化在 `jobs` 表的后台作业，类型：`env_alert_scan`（环境告警扫描）、`outbound_due_scan`（出库临期扫描）、`breeding_timeout_scan`（繁育超时扫描）、`restock_pending_scan`（回存验收超期扫描）。状态 `PENDING` → `RUNNING` → `DONE` / `FAILED`；失败按指数退避重排为 `PENDING`，超过 `max_attempts` 标记 `FAILED`。进程重启时崩溃遗留的 `RUNNING` 作业恢复为 `PENDING`，周期扫描作业入队去重，保证重启后持续存在。

## 业务链分步说明

1. **资源登记**：创建资源（`ACTIVE`）→ 在资源下登记 accession（`REGISTERED`）。归档后的资源不再接受新 accession。
2. **样本分装**：在 accession 下建立 `ORIGINAL` 原始批次（accession 自动转 `IN_STOCK`）→ 按数量列表分装为样本（总量不超过未分装可用量）。
3. **库位分配**：创建低温库位 → 将在库样本分配到库位（乐观锁校验，库位占用 +1）。
4. **环境监测**：按冷库注册温湿度传感器 → 持续写入读数（只增不改），供窗口评估与告警扫描使用。
5. **保存规则启用**：创建规则草稿版本（版本号自动递增）→ 启用（同 code 旧版本退役）。
6. **出库审批**：创建出库申请（支持幂等键）→ 审批：冻结样本/库位/规则/繁育目标，校验出库前环境窗口 → 可驳回、可取消（释放冻结）。
7. **繁育批次建立**：执行出库（校验审批后持续监控窗口）→ 基于已出库申请建立繁育计划（写入快照）。
8. **采样检测**：繁育期间追加田间观察（发芽率）→ 登记纯度检测（覆盖率、纯度，支持幂等键）。
9. **质量判定**：封存检测，按冻结规则计算 PASS/FAIL；一封存即只读，迟到检测不得覆盖。
10. **回存验收**：创建回存验收单 → 验收：复核封存合格结论 → 创建 RESTOCK 新批次、建立谱系、完成计划、关闭耗尽母批（同一事务，写快照）；或驳回（必填原因）。
11. **谱系关联**：按批次查询母批/子批；巡检自环、成环与孤儿批次。
12. **批次关闭**：批次随出库耗尽（`EXHAUSTED`）、回存验收关闭（`CLOSED`）或销毁审批（`DESTROYED`）进入终态。
