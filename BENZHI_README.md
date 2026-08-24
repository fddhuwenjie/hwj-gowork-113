# 本质评测环境说明

## 项目

- 项目编号：`hwj-gowork-113`
- 项目名称：农业种质资源低温保存、出库繁育与回存验收服务
- 项目说明：管理农业种质资源低温保存、出库审批、繁育检测、回存验收、谱系与后台巡检。

## 固定环境

- Go toolchain：`go1.26.5`
- go.mod language version：`go 1.21`
- GOTOOLCHAIN：`local`
- 支持平台：`linux/amd64`、`linux/arm64`
- Docker 基础镜像：`golang:1.26.5-bookworm`
- Docker manifest：`golang@sha256:53eeac89074db483fdf0ab3be1df32bf6e47562263d2d0d6baa7f26acb4957dd`

## 构建

评测镜像使用仓库内固定的 `benzhi.Dockerfile`，并通过 `build_benzhi_docker.sh` 构建：

```bash
./build_benzhi_docker.sh hwj-gowork-113:benzhi-amd64 linux/amd64
./build_benzhi_docker.sh hwj-gowork-113:benzhi-arm64 linux/arm64
```

## 运行

```bash
docker run --rm -it --network none hwj-gowork-113:benzhi-amd64 bash
```

## 容器内验证

```bash
go version
go env GOTOOLCHAIN GOPROXY GOMODCACHE GOCACHE
go test ./...
go vet ./...
go build ./...
```

---

# main 分支项目文档同步内容

---

## 来源：`README.md`

# 农业种质资源低温保存、出库繁育与回存验收服务

模块名 `germplasm`，是一个单进程 HTTP JSON 服务，基于 Go（go.mod 语言版本 1.21，工具链 go1.26.5，`GOTOOLCHAIN=local`）+ `database/sql` + 嵌入式 SQLite（`modernc.org/sqlite`，纯 Go 驱动、无需 CGO），全部数据持久化到环境变量 `DB_PATH` 指定的真实 SQLite 文件（禁止 `:memory:`）。

## 功能简介

服务管理农业种质资源在低温库中的全生命周期：资源与 accession 登记、批次建立、样本分装与库位分配、冷库温湿度采集、保存规则版本管理、出库申请审批（冻结样本/库位/规则/繁育目标）、繁育计划与田间观察、纯度检测与质量判定封存、回存验收（创建新批次并建立谱系）、批次销毁审批，以及后台周期扫描（环境告警、出库临期、繁育超时、回存超期）与风险巡检。

## 业务链

完整业务链路如下，每步详见 [docs/01-domain.md](docs/01-domain.md)：

1. **资源登记**：登记种质资源（`ACTIVE`）与 accession（种质编号）。
2. **样本分装**：在 accession 下建立原始批次（`ORIGINAL`），将批次可用数量分装为若干样本。
3. **库位分配**：将样本分配到低温库位（冷库/架/盒/格），库位占用 +1。
4. **环境监测**：注册温湿度传感器并持续写入读数（只增不改）。
5. **保存规则启用**：创建规则版本草稿（温湿度阈值、出库前后监控窗口、检测覆盖率与纯度门槛），启用后同 code 旧版本退役。
6. **出库审批**：创建出库申请（`PENDING`）；审批时在同一事务内按 FIFO 冻结样本与库位、冻结规则版本与繁育目标，并校验出库前环境时间窗覆盖。
7. **繁育批次建立**：出库（`FULFILLED`）后基于出库申请建立繁育计划（一个申请只能建一个计划）。
8. **采样检测**：繁育期间追加田间观察（发芽率等），登记纯度检测（覆盖率、纯度合格率）。
9. **质量判定**：封存检测时按规则计算合格/不合格结论；封存后结论只读，每个计划只能有一条封存结论，迟到检测不得覆盖。
10. **回存验收**：验收时复核封存结论必须合格，创建 `RESTOCK` 新批次、建立母批→子批谱系、完成繁育计划、关闭耗尽母批，全部在同一事务内完成。
11. **谱系关联**：通过谱系边查询任意批次的母批与子批，巡检自环/成环/孤儿批次异常。
12. **批次关闭**：批次经耗尽、回存关闭或销毁审批后进入 `CLOSED`/`DESTROYED` 终态。

## 技术栈

- 语言与工具链：Go（go.mod `go 1.21`，评测工具链 `go1.26.5`，`GOTOOLCHAIN=local`）
- HTTP：标准库 `net/http`（自研轻量路由 `internal/httpx/mux.go`）
- 数据库：`database/sql` + `modernc.org/sqlite` v1.29.10（WAL 模式、外键开启、单连接串行化写入）
- 日志：`log/slog` JSON 结构化日志（stderr）
- 测试：标准库 `testing` + `net/http/httptest`（`internal/itest` 集成测试）

## 目录结构

| 路径 | 职责 |
| --- | --- |
| `cmd/server` | 服务入口：加载配置、打开 SQLite、组装服务、启动作业调度与 HTTP、优雅关闭 |
| `internal/config` | 从环境变量加载配置（PORT、DB_PATH 等）并校验 |
| `internal/clock` | 可注入时钟：`Real`（UTC 系统时间）与 `Fake`（测试手动推进） |
| `internal/logging` | 基于 `log/slog` 的 JSON 结构化日志器 |
| `internal/apperr` | 统一应用错误与错误码，映射 HTTP 状态与统一错误响应体 |
| `internal/domain` | 领域模型、状态机（合法状态转换表）、数量守恒与环境窗口等核心规则 |
| `internal/store` | SQLite 连接（WAL/外键/忙等）、全量建表迁移、事务管理器 |
| `internal/repository` | 各实体持久化仓储与稳定游标分页 |
| `internal/service` | 业务用例编排：事务、幂等、乐观锁、快照与审计 |
| `internal/audit` | 审计日志写入与分页查询（与业务写入同一事务） |
| `internal/jobs` | 持久化后台作业调度器与扫描作业处理器，支持重启恢复 |
| `internal/httpx` | HTTP 层：路由、中间件（访问日志/panic 恢复）、统一错误、分页参数 |
| `internal/itest` | 集成测试（业务流、幂等、乐观锁、分页、环境窗、质量、谱系、作业等） |

## 快速开始

### 环境变量

| 变量 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- |
| `PORT` | 否 | `8080` | HTTP 监听端口 |
| `DB_PATH` | **是** | 无 | SQLite 数据库文件路径（不得为 `:memory:`） |

更多环境变量（优雅关闭、作业调度、日志级别等）见 [docs/05-deploy.md](docs/05-deploy.md)。

### 本地运行

```bash
# 构建
go build ./...

# 运行（DB_PATH 必填）
DB_PATH=./data/germplasm.db PORT=8080 go run ./cmd/server

# 健康检查
curl http://localhost:8080/healthz

# 登记一份种质资源
curl -X POST http://localhost:8080/api/v1/resources \
  -H 'Content-Type: application/json' \
  -H 'X-Actor: admin' \
  -d '{"code":"RES001","name":"水稻种质","species":"Oryza sativa","category":"粮食作物"}'

# 运行全部测试
go test ./...
```

### Docker 构建

仓库提供 `Dockerfile`（与 `benzhi.Dockerfile` 逐字一致）与构建脚本 `build_docker.sh`；评测环境使用 `benzhi.Dockerfile` 与 `build_benzhi_docker.sh`。两套均基于 `golang@sha256:53eeac89074db483fdf0ab3be1df32bf6e47562263d2d0d6baa7f26acb4957dd`，`GOTOOLCHAIN=local`，go1.26.5，支持 `linux/amd64` 与 `linux/arm64` 双架构：

```bash
./build_docker.sh hwj-gowork-113:amd64 linux/amd64
./build_docker.sh hwj-gowork-113:arm64 linux/arm64
```

```bash
./build_benzhi_docker.sh hwj-gowork-113:benzhi-amd64 linux/amd64
./build_benzhi_docker.sh hwj-gowork-113:benzhi-arm64 linux/arm64
```

容器内无网络验收：

```bash
docker run --rm -it --network none hwj-gowork-113:benzhi-amd64 bash
# 容器内执行
go test ./...
go vet ./...
go build ./...
```

详细部署说明见 [docs/05-deploy.md](docs/05-deploy.md)。

## 核心设计要点

- **出库冻结**：审批出库申请时在同一事务内冻结样本数量（必要时拆分样本）、库位、保存规则版本与繁育目标，此后不可更改；取消已审批申请会整体释放冻结。
- **温湿度时间窗覆盖**：出库审批校验出库前窗口、执行出库校验审批后持续监控窗口；窗口按小时分桶，桶内有读数且全部达标记为覆盖，任何越限读数即失败（`ENV_WINDOW_VIOLATION`）。
- **数量守恒**：批次满足 `qty_total = qty_available + qty_frozen + qty_outbound + qty_destroyed`；分装、冻结、出库、销毁、回存均不产生也不消灭数量，破坏守恒即报 `QUANTITY_VIOLATION`。
- **检测封存与迟到检测**：纯度检测封存后结论只读；每个繁育计划只允许一条封存结论；`tested_at` 早于封存时刻的迟到检测仅作只读记录，不得参与判定。
- **幂等键**：出库申请、纯度检测、回存验收单的创建接口支持 `idempotency_key`，重复提交返回首个实体；同键不同请求体报 `IDEMPOTENCY_CONFLICT`。
- **乐观锁**：资源、批次、样本、出库申请、繁育计划、检测、回存单、销毁单等均带 `version` 列，状态变更请求体须携带当前 `version`，不匹配报 `OPTIMISTIC_LOCK`。
- **SQLite 事务回滚**：所有多步写入经 `TxManager.InTx` 执行，任何一步失败（含提交失败）整体回滚，审计与业务写入同生共死。
- **历史快照**：出库审批/出库、繁育计划建立/关闭、检测封存、回存验收/驳回等关键事件将当时实体状态序列化存入 `snapshots` 表。
- **后台作业重启恢复**：作业持久化在 `jobs` 表，进程启动时把崩溃遗留的 `RUNNING` 作业重置为 `PENDING`；失败按指数退避重试，超过最大尝试次数标记 `FAILED`；周期扫描作业入队去重。
- **稳定游标分页**：列表接口按 `(created_at, id)` 升序游标翻页（ID 由时间戳+单调计数器+随机数构成，字典序与创建顺序一致），响应为 `items` + `next_cursor`，翻页不重不漏；审计日志按自增 ID 游标。
- **统一错误 JSON**：所有错误响应为 `{"error":{"code","message","details"}}`，错误码见 [docs/04-api.md](docs/04-api.md)。
- **结构化日志**：`log/slog` JSON 输出到 stderr，每个 HTTP 请求记录方法、路径、状态码、耗时与操作者（`X-Actor` 头）。
- **优雅关闭**：收到 SIGINT/SIGTERM 后先停作业调度器，再在 `SHUTDOWN_TIMEOUT_SECONDS` 内关闭 HTTP 服务并关闭数据库。
- **可注入时钟**：业务时间全部来自注入的 `clock.Clock`，测试用 `Fake` 时钟可精确驱动超时、窗口与扫描场景。

## 文档索引

- [docs/01-domain.md](docs/01-domain.md) — 领域概念与业务链说明
- [docs/02-states.md](docs/02-states.md) — 状态转换表
- [docs/03-model.md](docs/03-model.md) — 数据模型（表结构、约束、索引）
- [docs/04-api.md](docs/04-api.md) — HTTP 接口契约
- [docs/05-deploy.md](docs/05-deploy.md) — 部署与运行说明

---

## 来源：`docs/01-domain.md`

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

---

## 来源：`docs/02-states.md`

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

---

## 来源：`docs/03-model.md`

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

---

## 来源：`docs/04-api.md`

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

---

## 来源：`docs/05-deploy.md`

# 05 部署说明

## 固定环境

- Go 工具链：`go1.26.5`（`GOTOOLCHAIN=local`，不自动下载其他工具链）
- go.mod 语言版本：`go 1.21`
- 支持平台：`linux/amd64`、`linux/arm64`
- Docker 基础镜像：`golang@sha256:53eeac89074db483fdf0ab3be1df32bf6e47562263d2d0d6baa7f26acb4957dd`
- 数据库：嵌入式 SQLite（`modernc.org/sqlite`，纯 Go 驱动，无需 CGO），持久化到 `DB_PATH` 指定的真实文件

## 环境变量

| 变量 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `PORT` | 否 | `8080` | HTTP 监听端口（1~65535） |
| `DB_PATH` | **是** | 无 | SQLite 数据库文件路径；不得为空，不得为 `:memory:`（服务必须持久化到真实文件） |
| `SHUTDOWN_TIMEOUT_SECONDS` | 否 | `10` | 优雅关闭等待时间（秒） |
| `JOB_INTERVAL_MS` | 否 | `500` | 后台作业调度轮询间隔（毫秒，最小 50） |
| `JOB_SCAN_INTERVAL_SECONDS` | 否 | `30` | 周期扫描类作业的入队间隔（秒） |
| `OUTBOUND_DUE_SOON_HOURS` | 否 | `24` | 出库临期阈值（小时）：距交付截止不足该值即产生临期告警 |
| `RESTOCK_PENDING_HOURS` | 否 | `72` | 回存验收超期阈值（小时）：创建超过该值仍待验收即告警 |
| `JOB_MAX_ATTEMPTS` | 否 | `5` | 失败作业最大重试次数（超过标记 FAILED） |
| `LOG_LEVEL` | 否 | `info` | 结构化日志级别：`debug` / `info` / `warn` / `error` |

配置在启动时加载并校验，非法取值（如端口超界、`DB_PATH` 为空）会导致进程启动失败并以非零码退出。

## 本地运行

```bash
# 构建
go build ./...

# 运行（DB_PATH 必填；目录不存在会自动创建）
DB_PATH=./data/germplasm.db PORT=8080 go run ./cmd/server

# 健康检查
curl http://localhost:8080/healthz
```

启动流程：加载配置 → 初始化 JSON 日志 → 打开（必要时创建）SQLite 文件并执行全量迁移 → 组装领域服务 → 恢复崩溃中断的作业并启动调度器 → 启动 HTTP 监听。

运行测试与静态检查：

```bash
go test ./...
go vet ./...
go build ./...
```

## Docker 构建

仓库提供 `Dockerfile`（与 `benzhi.Dockerfile` 逐字一致）与构建脚本 `build_docker.sh`；评测环境使用 `benzhi.Dockerfile` 与 `build_benzhi_docker.sh`。两套均支持 `linux/amd64` 与 `linux/arm64` 双架构：

```bash
./build_docker.sh hwj-gowork-113:amd64 linux/amd64
./build_docker.sh hwj-gowork-113:arm64 linux/arm64
```

```bash
./build_benzhi_docker.sh hwj-gowork-113:benzhi-amd64 linux/amd64
./build_benzhi_docker.sh hwj-gowork-113:benzhi-arm64 linux/arm64
```

说明：

- `build_docker.sh` 调用 `docker buildx build --platform <平台> -f Dockerfile --load`，`build_benzhi_docker.sh` 使用 `-f benzhi.Dockerfile`，两者均可用环境变量 `BUILDER` 指定构建器（默认 `default`）；
- 镜像以 `golang@sha256:53eeac89074db483fdf0ab3be1df32bf6e47562263d2d0d6baa7f26acb4957dd` 为基础，内置 go1.26.5 工具链，设置 `GOTOOLCHAIN=local`、`GOMODCACHE=/go/pkg/mod`、`GOCACHE=/tmp/go-build`；
- 构建期先 `go mod download` 拉取依赖，再复制源码并在 `/tmp/source-check` 中执行 `go build ./...` 验证可编译。

## 容器内无网络验收

```bash
docker run --rm -it --network none hwj-gowork-113:benzhi-amd64 bash
```

容器内执行：

```bash
go version                       # 应为 go1.26.5
go env GOTOOLCHAIN GOPROXY GOMODCACHE GOCACHE
go test ./...
go vet ./...
go build ./...
```

依赖已在构建期下载至镜像内 `GOMODCACHE`，以上命令在 `--network none` 下可全部通过。

## 数据持久化

- 服务以 WAL（Write-Ahead Logging）模式打开 SQLite：DSN 携带 `_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)`；
- 数据库连接数限制为 1（`SetMaxOpenConns(1)`），写操作串行化以避免 `SQLITE_BUSY`；
- `DB_PATH` 的父目录不存在时自动创建（`0755`）；
- 容器化部署时应将 `DB_PATH` 所在目录挂载为数据卷，例如：

```bash
docker run -d --name germplasm \
  -e DB_PATH=/data/germplasm.db -e PORT=8080 \
  -v germplasm-data:/data -p 8080:8080 \
  <镜像名> ./server
```

（镜像默认 `CMD ["bash"]`，用于评测验收；运行服务需另行编译或以 `go run ./cmd/server` 启动。）

## 优雅关闭与后台作业重启恢复

- 进程监听 `SIGINT` / `SIGTERM`：收到信号后先停止作业调度器（等待轮询与入队循环退出），再在 `SHUTDOWN_TIMEOUT_SECONDS` 内优雅关闭 HTTP 服务（等待在途请求完成），最后关闭数据库连接；
- 后台作业持久化在 `jobs` 表：进程启动时执行恢复，把崩溃遗留的 `RUNNING` 作业全部重置为 `PENDING` 等待重新执行；失败作业按指数退避（`2^(attempts-1)` 秒）重排，超过 `JOB_MAX_ATTEMPTS` 标记 `FAILED`；四类周期扫描作业（环境告警、出库临期、繁育超时、回存超期）按 `JOB_SCAN_INTERVAL_SECONDS` 去重入队，保证重启后扫描任务持续存在；
- 所有多步写入在事务内完成，任何一步失败整体回滚，重启后不会产生半成品状态。
