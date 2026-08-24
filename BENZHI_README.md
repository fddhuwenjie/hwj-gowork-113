# 本质评测环境说明

## 项目

- 项目编号：`hwj-gowork-113`
- 项目名称：农业种质资源低温保存、出库繁育与回存验收服务
- 项目说明：基于 Go HTTP JSON 与 SQLite 文件持久化，管理种质资源批次、低温库位、出库繁育、质量检测、谱系和回存验收。

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
