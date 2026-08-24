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
