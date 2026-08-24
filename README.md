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
