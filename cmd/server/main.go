// Command server 为农业种质资源低温保存、出库繁育与回存验收服务入口。
// 读取环境变量 PORT 与 DB_PATH，持久化到真实 SQLite 文件，提供
// /healthz、统一错误、结构化日志、优雅关闭与可注入时钟。
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"germplasm/internal/audit"
	"germplasm/internal/clock"
	"germplasm/internal/config"
	"germplasm/internal/httpx"
	"germplasm/internal/jobs"
	"germplasm/internal/logging"
	"germplasm/internal/service"
	"germplasm/internal/store"
)

func main() {
	if err := run(); err != nil {
		// 启动失败直接输出并退出非零。
		println("服务启动失败: " + err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logging.New(cfg.LogLevel)
	ctx := context.Background()

	// 打开真实 SQLite 文件并执行迁移。
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	log.Info("数据库已就绪", "path", cfg.DBPath)

	// 组装领域服务与依赖。
	clk := clock.Real{}
	txMgr := store.NewTxManager(db)
	repos := service.NewRepos()
	svc := service.New(txMgr, clk, audit.NewWriter(), repos)

	// 后台作业：先恢复崩溃中断的作业，再启动调度。
	scheduler := jobs.NewScheduler(svc, cfg, clk, log)
	if err := scheduler.Recover(ctx); err != nil {
		return err
	}
	scheduler.Start(ctx)

	// 启动 HTTP 服务。
	server := httpx.NewServer(cfg.Port, svc, db, clk, log)
	errCh := make(chan error, 1)
	server.Start(errCh)

	// 等待退出信号或启动错误。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		log.Info("收到退出信号，开始优雅关闭", "signal", sig.String())
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	scheduler.Stop()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("HTTP 优雅关闭失败", "error", err)
	}
	if err := db.Close(); err != nil {
		log.Error("关闭数据库失败", "error", err)
	}
	log.Info("服务已停止", "time", time.Now().UTC().Format(time.RFC3339Nano))
	return nil
}
