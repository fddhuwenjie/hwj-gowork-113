# 03 数据模型

本文档按 `internal/store/migrate.go` 的真实建表语句逐表说明字段、类型、约束与索引。通用约定：

- 除 `sensor_readings`、`snapshots`、`audit_logs` 使用 `INTEGER PRIMARY KEY AUTOINCREMENT` 外，各表主键 `id` 为带前缀的文本 ID（如 `bat_...`，由时间戳 + 单调计数器 + 随机数构成，字典序与创建顺序一致）；
- 时间列以 RFC3339Nano 文本（UTC）存储；
- 多数业务表带 `version INTEGER NOT NULL DEFAULT 1` 乐观锁列：状态变更的 UPDATE 以 `id + version` 为条件，影响行数为 0 即冲突；
- 迁移语句均为 `CREATE TABLE/INDEX IF NOT EXISTS`，启动时全量执行，重复执行安全。

## 通用机制

### 数量守恒公式

批次表 `batches` 必须满足：

```
qty_total = qty_available + qty_frozen + qty_outbound + qty_destroyed
```

分装、冻结、出库、销毁、回存等所有数量变动都在四个分项间转移，不产生也不消灭数量；`domain.Batch.CheckConservation()` 返回偏差（0 表示守恒），库存差异巡检以此核对账面与样本汇总。

### 幂等键唯一约束

`outbound_requests`、`purity_tests`、`restock_batches` 三张表带 `idempotency_key TEXT UNIQUE` 列；另有独立的 `idempotency_keys` 表以 `(key, endpoint)` 为主键，记录请求哈希、响应与状态码。同键同请求体重放返回首个实体，同键不同请求体报 `IDEMPOTENCY_CONFLICT`。

### 乐观锁 version 列

`resources`、`accessions`、`batches`、`samples`、`locations`、`outbound_requests`、`breeding_plans`、`purity_tests`、`restock_batches`、`destruction_approvals`、`jobs` 均带 `version` 列。写操作请求体须携带当前 `version`，服务层先比对再执行 `UPDATE ... WHERE id=? AND version=?`，不匹配报 `OPTIMISTIC_LOCK`。

## 表结构

### resources — 种质资源

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 资源 ID（`res_` 前缀） |
| code | TEXT | NOT NULL UNIQUE | 业务编码，全局唯一 |
| name | TEXT | NOT NULL | 名称 |
| species | TEXT | NOT NULL | 物种 |
| category | TEXT | NOT NULL | 类别（粮食作物/蔬菜/果树等） |
| status | TEXT | NOT NULL | ACTIVE / ARCHIVED |
| remark | TEXT | NOT NULL DEFAULT '' | 备注 |
| version | INTEGER | NOT NULL DEFAULT 1 | 乐观锁版本 |
| created_at / updated_at | TEXT | NOT NULL | 创建/更新时间 |

### accessions — 种质编号登记

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | accession ID（`acc_` 前缀） |
| resource_id | TEXT | NOT NULL REFERENCES resources(id) | 所属资源 |
| accession_no | TEXT | NOT NULL UNIQUE | 种质编号，全局唯一 |
| origin | TEXT | NOT NULL DEFAULT '' | 产地 |
| donor | TEXT | NOT NULL DEFAULT '' | 供种者 |
| collected_at | TEXT | NOT NULL DEFAULT '' | 采集时间 |
| status | TEXT | NOT NULL | REGISTERED / IN_STOCK / DEPLETED |
| version | INTEGER | NOT NULL DEFAULT 1 | 乐观锁版本 |
| created_at / updated_at | TEXT | NOT NULL | 创建/更新时间 |

索引：`idx_accessions_resource ON accessions(resource_id, id)`（按资源分页查询 accession）。

### batches — 批次

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 批次 ID（`bat_` 前缀） |
| accession_id | TEXT | NOT NULL REFERENCES accessions(id) | 所属 accession |
| batch_no | TEXT | NOT NULL UNIQUE | 批次编号，全局唯一 |
| kind | TEXT | NOT NULL | ORIGINAL / REGENERATION / RESTOCK |
| mother_batch_id | TEXT | NOT NULL DEFAULT '' | 母批 ID（回存/繁育批次） |
| unit | TEXT | NOT NULL DEFAULT '粒' | 计量单位 |
| qty_total | INTEGER | NOT NULL | 总量（守恒公式左侧） |
| qty_available | INTEGER | NOT NULL | 可用量 |
| qty_frozen | INTEGER | NOT NULL DEFAULT 0 | 冻结量 |
| qty_outbound | INTEGER | NOT NULL DEFAULT 0 | 已出库量 |
| qty_destroyed | INTEGER | NOT NULL DEFAULT 0 | 已销毁量 |
| status | TEXT | NOT NULL | ACTIVE / EXHAUSTED / CLOSED / DESTROYED |
| version | INTEGER | NOT NULL DEFAULT 1 | 乐观锁版本 |
| created_at / updated_at | TEXT | NOT NULL | 创建/更新时间 |
| closed_at | TEXT | 可空 | 关闭时间 |

索引：`idx_batches_accession ON batches(accession_id, id)`。

### samples — 样本

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 样本 ID（`sam_` 前缀） |
| batch_id | TEXT | NOT NULL REFERENCES batches(id) | 所属批次 |
| sample_no | TEXT | NOT NULL UNIQUE | 样本编号（拆分样本加 `-F`/`-D` 后缀） |
| qty | INTEGER | NOT NULL | 数量 |
| status | TEXT | NOT NULL | IN_STOCK / FROZEN / OUTBOUND / DESTROYED |
| location_id | TEXT | NOT NULL DEFAULT '' | 所在库位（未分配为空） |
| version | INTEGER | NOT NULL DEFAULT 1 | 乐观锁版本 |
| created_at / updated_at | TEXT | NOT NULL | 创建/更新时间 |

索引：`idx_samples_batch ON samples(batch_id, status, created_at)`（支持按批次+状态的 FIFO 选取与汇总）。

### locations — 低温库位

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 库位 ID（`loc_` 前缀） |
| code | TEXT | NOT NULL UNIQUE | 库位编码（如 C01-R02-B03-S04） |
| chamber / rack / box / slot | TEXT | NOT NULL | 冷库/架/盒/格 |
| capacity | INTEGER | NOT NULL | 可容纳样本数 |
| occupied | INTEGER | NOT NULL DEFAULT 0 | 已占用样本数 |
| status | TEXT | NOT NULL | IDLE / ACTIVE / DISABLED |
| version | INTEGER | NOT NULL DEFAULT 1 | 乐观锁版本 |
| created_at / updated_at | TEXT | NOT NULL | 创建/更新时间 |

索引：`idx_locations_chamber ON locations(chamber, id)`。占用与释放通过条件 UPDATE 保证不超容（`occupied < capacity AND status != 'DISABLED'`）。

### sensors — 环境传感器

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 传感器 ID（`sen_` 前缀） |
| code | TEXT | NOT NULL UNIQUE | 传感器编码 |
| chamber | TEXT | NOT NULL | 所在冷库 |
| metric | TEXT | NOT NULL | TEMPERATURE / HUMIDITY |
| status | TEXT | NOT NULL | ONLINE / OFFLINE |
| created_at | TEXT | NOT NULL | 创建时间 |

索引：`idx_sensors_chamber ON sensors(chamber, metric)`。

### sensor_readings — 环境读数

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 自增 ID |
| sensor_id | TEXT | NOT NULL REFERENCES sensors(id) | 所属传感器 |
| metric | TEXT | NOT NULL | 度量类型（与传感器一致） |
| value | REAL | NOT NULL | 读数值 |
| recorded_at | TEXT | NOT NULL | 采样时间 |
| created_at | TEXT | NOT NULL | 写入时间 |

索引：`idx_readings_sensor ON sensor_readings(sensor_id, recorded_at)`（窗口查询）。读数只增不改。

### rule_versions — 保存规则版本

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 规则版本 ID（`rul_` 前缀） |
| code | TEXT | NOT NULL | 规则编码 |
| version_no | INTEGER | NOT NULL | 版本号（同 code 递增） |
| min_temp / max_temp | REAL | NOT NULL | 温度阈值 |
| min_humidity / max_humidity | REAL | NOT NULL | 湿度阈值 |
| window_before_hours | INTEGER | NOT NULL | 出库前监控窗口（小时） |
| window_after_hours | INTEGER | NOT NULL | 出库后监控窗口（小时） |
| min_coverage | REAL | NOT NULL | 检测覆盖率门槛（0~1） |
| min_purity | REAL | NOT NULL | 纯度合格率门槛（0~1） |
| status | TEXT | NOT NULL | DRAFT / ACTIVE / RETIRED |
| effective_from | TEXT | 可空 | 生效时间 |
| created_at | TEXT | NOT NULL | 创建时间 |

约束：`UNIQUE(code, version_no)`。索引：`idx_rules_status ON rule_versions(code, status)`（查同 code 的 ACTIVE 版本）。

### outbound_requests — 出库申请

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 申请 ID（`out_` 前缀） |
| request_no | TEXT | NOT NULL UNIQUE | 申请编号 |
| accession_id | TEXT | NOT NULL REFERENCES accessions(id) | 目标 accession |
| batch_id | TEXT | NOT NULL REFERENCES batches(id) | 目标批次 |
| qty | INTEGER | NOT NULL | 出库数量 |
| purpose | TEXT | NOT NULL DEFAULT '' | 用途 |
| breeding_target | TEXT | NOT NULL DEFAULT '' | 繁育目标（审批后冻结） |
| rule_version_id | TEXT | NOT NULL REFERENCES rule_versions(id) | 保存规则版本（审批后冻结） |
| deadline | TEXT | NOT NULL | 交付截止时间 |
| status | TEXT | NOT NULL | PENDING / APPROVED / REJECTED / FULFILLED / CANCELLED |
| idempotency_key | TEXT | UNIQUE | 幂等键（可空） |
| version | INTEGER | NOT NULL DEFAULT 1 | 乐观锁版本 |
| created_at / updated_at | TEXT | NOT NULL | 创建/更新时间 |

索引：`idx_outbound_status ON outbound_requests(status, deadline)`（出库临期扫描查 APPROVED 且临期）。

### outbound_freezes — 出库冻结明细

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 冻结明细 ID（`frz_` 前缀） |
| request_id | TEXT | NOT NULL REFERENCES outbound_requests(id) | 所属申请 |
| sample_id | TEXT | NOT NULL REFERENCES samples(id) | 冻结样本 |
| location_id | TEXT | NOT NULL DEFAULT '' | 样本所在库位 |
| qty | INTEGER | NOT NULL | 冻结数量 |
| status | TEXT | NOT NULL | ACTIVE / RELEASED / CONSUMED |
| created_at | TEXT | NOT NULL | 创建时间 |

索引：`idx_freezes_request ON outbound_freezes(request_id, status)`。

### breeding_plans — 繁育计划

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 计划 ID（`pln_` 前缀） |
| plan_no | TEXT | NOT NULL UNIQUE | 计划编号 |
| outbound_request_id | TEXT | NOT NULL UNIQUE REFERENCES outbound_requests(id) | 出库申请（一申请一计划） |
| batch_id | TEXT | NOT NULL REFERENCES batches(id) | 母批 |
| target_qty | INTEGER | NOT NULL | 繁育目标数量 |
| plot | TEXT | NOT NULL DEFAULT '' | 田块 |
| deadline | TEXT | NOT NULL | 繁育期限 |
| status | TEXT | NOT NULL | ACTIVE / COMPLETED / TIMEOUT / CLOSED |
| version | INTEGER | NOT NULL DEFAULT 1 | 乐观锁版本 |
| created_at / updated_at | TEXT | NOT NULL | 创建/更新时间 |

索引：`idx_plans_status ON breeding_plans(status, deadline)`（繁育超时扫描）。

### field_observations — 田间观察

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 观察记录 ID（`obs_` 前缀） |
| plan_id | TEXT | NOT NULL REFERENCES breeding_plans(id) | 所属计划 |
| observed_at | TEXT | NOT NULL | 观察时间 |
| germination_rate | REAL | NOT NULL | 发芽率（0~1） |
| vigor | TEXT | NOT NULL DEFAULT '' | 长势 |
| notes | TEXT | NOT NULL DEFAULT '' | 备注 |
| created_at | TEXT | NOT NULL | 创建时间 |

索引：`idx_observations_plan ON field_observations(plan_id, observed_at)`。

### purity_tests — 纯度检测

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 检测 ID（`pts_` 前缀） |
| plan_id | TEXT | NOT NULL REFERENCES breeding_plans(id) | 所属计划 |
| sample_qty | INTEGER | NOT NULL | 抽样数量 |
| coverage_ratio | REAL | NOT NULL | 检测覆盖率（0~1） |
| purity_rate | REAL | NOT NULL | 纯度合格率（0~1） |
| verdict | TEXT | NOT NULL | PENDING / PASS / FAIL |
| sealed | INTEGER | NOT NULL DEFAULT 0 | 是否封存（0/1） |
| sealed_at | TEXT | 可空 | 封存时间 |
| tested_at | TEXT | NOT NULL | 检测时间 |
| idempotency_key | TEXT | UNIQUE | 幂等键（可空） |
| version | INTEGER | NOT NULL DEFAULT 1 | 乐观锁版本 |
| created_at | TEXT | NOT NULL | 创建时间 |

索引：`idx_tests_plan ON purity_tests(plan_id, sealed)`（查计划最新封存结论）。

### restock_batches — 回存验收单

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 验收单 ID（`rst_` 前缀） |
| request_no | TEXT | NOT NULL UNIQUE | 回存单号 |
| plan_id | TEXT | NOT NULL REFERENCES breeding_plans(id) | 繁育计划 |
| qty | INTEGER | NOT NULL | 回存数量 |
| status | TEXT | NOT NULL | PENDING / ACCEPTED / REJECTED |
| new_batch_id | TEXT | NOT NULL DEFAULT '' | 验收通过创建的新批次 |
| reject_reason | TEXT | NOT NULL DEFAULT '' | 驳回原因 |
| idempotency_key | TEXT | UNIQUE | 幂等键（可空） |
| version | INTEGER | NOT NULL DEFAULT 1 | 乐观锁版本 |
| created_at / updated_at | TEXT | NOT NULL | 创建/更新时间 |

索引：`idx_restock_status ON restock_batches(status, created_at)`（回存超期扫描与待回存巡检）。

### destruction_approvals — 销毁审批

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 审批单 ID（`dst_` 前缀） |
| batch_id | TEXT | NOT NULL REFERENCES batches(id) | 目标批次 |
| qty | INTEGER | NOT NULL | 销毁数量 |
| reason | TEXT | NOT NULL DEFAULT '' | 销毁原因 |
| status | TEXT | NOT NULL | PENDING / APPROVED / REJECTED |
| approver | TEXT | NOT NULL DEFAULT '' | 审批人 |
| version | INTEGER | NOT NULL DEFAULT 1 | 乐观锁版本 |
| created_at / updated_at | TEXT | NOT NULL | 创建/更新时间 |

### lineage_edges — 资源谱系边

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 谱系边 ID（`lin_` 前缀） |
| resource_id | TEXT | NOT NULL REFERENCES resources(id) | 所属资源 |
| parent_batch_id | TEXT | NOT NULL REFERENCES batches(id) | 母批 |
| child_batch_id | TEXT | NOT NULL REFERENCES batches(id) | 子批 |
| relation | TEXT | NOT NULL | RESTOCK / REGENERATION |
| created_at | TEXT | NOT NULL | 创建时间 |

约束：`UNIQUE(parent_batch_id, child_batch_id, relation)`。索引：`idx_lineage_child ON lineage_edges(child_batch_id)`、`idx_lineage_parent ON lineage_edges(parent_batch_id)`（双向查询母批/子批）。

### snapshots — 历史快照

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 自增 ID |
| entity_type | TEXT | NOT NULL | 实体类型（outbound / breeding_plan / purity_test / restock 等） |
| entity_id | TEXT | NOT NULL | 实体 ID |
| event | TEXT | NOT NULL | 事件（APPROVED / FULFILLED / CREATED / CLOSED / SEALED / ACCEPTED / REJECTED 等） |
| payload | TEXT | NOT NULL | 事件时刻实体状态的 JSON 序列化 |
| created_at | TEXT | NOT NULL | 创建时间 |

索引：`idx_snapshots_entity ON snapshots(entity_type, entity_id, id)`。

### idempotency_keys — 幂等记录

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| key | TEXT | 复合主键 | 幂等键 |
| endpoint | TEXT | 复合主键 | 接口标识（如 outbound.create / purity.create / restock.create） |
| request_hash | TEXT | NOT NULL | 请求体哈希 |
| response | TEXT | NOT NULL | 首个响应（含实体 ID） |
| status_code | INTEGER | NOT NULL | 首个响应状态码 |
| created_at | TEXT | NOT NULL | 创建时间 |

主键：`PRIMARY KEY(key, endpoint)`。

### jobs — 后台作业

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 作业 ID（`job_` 前缀） |
| type | TEXT | NOT NULL | env_alert_scan / outbound_due_scan / breeding_timeout_scan / restock_pending_scan |
| payload | TEXT | NOT NULL DEFAULT '{}' | 负载 JSON |
| status | TEXT | NOT NULL | PENDING / RUNNING / DONE / FAILED |
| attempts | INTEGER | NOT NULL DEFAULT 0 | 已尝试次数 |
| max_attempts | INTEGER | NOT NULL DEFAULT 5 | 最大尝试次数 |
| next_run_at | TEXT | NOT NULL | 下次执行时间（失败按指数退避重排） |
| last_error | TEXT | NOT NULL DEFAULT '' | 最近错误 |
| version | INTEGER | NOT NULL DEFAULT 1 | 乐观锁版本（抢占防并发） |
| created_at / updated_at | TEXT | NOT NULL | 创建/更新时间 |

索引：`idx_jobs_poll ON jobs(status, next_run_at)`（调度器轮询到期作业）。

### alerts — 告警

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | TEXT | PRIMARY KEY | 告警 ID（`alt_` 前缀） |
| type | TEXT | NOT NULL | ENV_OUT_OF_RANGE / OUTBOUND_DUE_SOON / BREEDING_TIMEOUT / RESTOCK_PENDING |
| ref_type | TEXT | NOT NULL | 关联实体类型（sensor / outbound_request / breeding_plan / restock_batch） |
| ref_id | TEXT | NOT NULL | 关联实体 ID |
| message | TEXT | NOT NULL | 告警内容 |
| status | TEXT | NOT NULL | OPEN / ACKED |
| created_at | TEXT | NOT NULL | 创建时间 |
| acked_at | TEXT | 可空 | 确认时间 |

索引：`idx_alerts_open ON alerts(status, type, ref_type, ref_id)`（同源 OPEN 告警去重）。

### audit_logs — 审计日志

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 自增 ID（分页游标） |
| actor | TEXT | NOT NULL | 操作者（X-Actor 头，缺省 system） |
| action | TEXT | NOT NULL | 动作（如 outbound.approve） |
| entity_type | TEXT | NOT NULL | 实体类型 |
| entity_id | TEXT | NOT NULL | 实体 ID |
| detail | TEXT | NOT NULL DEFAULT '' | 明细 JSON |
| created_at | TEXT | NOT NULL | 创建时间 |

索引：`idx_audit_entity ON audit_logs(entity_type, entity_id, id)`。审计日志在业务事务内写入，与业务写入同生共死。
