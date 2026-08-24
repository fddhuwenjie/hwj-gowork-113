// Package config 负责从环境变量加载服务配置。
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config 汇总服务运行所需的全部配置项。
type Config struct {
	// Port 为 HTTP 监听端口，来自环境变量 PORT，默认 8080。
	Port int
	// DBPath 为 SQLite 数据库文件路径，来自环境变量 DB_PATH，必填。
	DBPath string
	// ShutdownTimeout 为优雅关闭等待时间。
	ShutdownTimeout time.Duration
	// JobInterval 为后台作业调度轮询间隔。
	JobInterval time.Duration
	// JobScanInterval 为周期扫描类作业的入队间隔。
	JobScanInterval time.Duration
	// OutboundDueSoonHours 出库临期阈值（小时）：距交付截止时间不足该值即产生临期告警。
	OutboundDueSoonHours int
	// RestockPendingHours 回存验收超期阈值（小时）。
	RestockPendingHours int
	// JobMaxAttempts 失败作业最大重试次数。
	JobMaxAttempts int
	// LogLevel 结构化日志级别（debug/info/warn/error）。
	LogLevel string
}

// Load 从环境变量读取配置并做基础校验。
func Load() (Config, error) {
	cfg := Config{
		Port:                 8080,
		DBPath:               os.Getenv("DB_PATH"),
		ShutdownTimeout:      10 * time.Second,
		JobInterval:          500 * time.Millisecond,
		JobScanInterval:      30 * time.Second,
		OutboundDueSoonHours: 24,
		RestockPendingHours:  72,
		JobMaxAttempts:       5,
		LogLevel:             "info",
	}
	if cfg.DBPath == "" {
		return Config{}, fmt.Errorf("环境变量 DB_PATH 不能为空，且不得使用 :memory:")
	}
	if cfg.DBPath == ":memory:" {
		return Config{}, fmt.Errorf("DB_PATH 不得为 :memory:，服务必须持久化到真实 SQLite 文件")
	}
	if v := os.Getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p <= 0 || p > 65535 {
			return Config{}, fmt.Errorf("环境变量 PORT 非法: %q", v)
		}
		cfg.Port = p
	}
	if v := os.Getenv("SHUTDOWN_TIMEOUT_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT_SECONDS 非法: %q", v)
		}
		cfg.ShutdownTimeout = time.Duration(n) * time.Second
	}
	if v := os.Getenv("JOB_INTERVAL_MS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 50 {
			return Config{}, fmt.Errorf("JOB_INTERVAL_MS 非法: %q", v)
		}
		cfg.JobInterval = time.Duration(n) * time.Millisecond
	}
	if v := os.Getenv("JOB_SCAN_INTERVAL_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("JOB_SCAN_INTERVAL_SECONDS 非法: %q", v)
		}
		cfg.JobScanInterval = time.Duration(n) * time.Second
	}
	if v := os.Getenv("OUTBOUND_DUE_SOON_HOURS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("OUTBOUND_DUE_SOON_HOURS 非法: %q", v)
		}
		cfg.OutboundDueSoonHours = n
	}
	if v := os.Getenv("RESTOCK_PENDING_HOURS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("RESTOCK_PENDING_HOURS 非法: %q", v)
		}
		cfg.RestockPendingHours = n
	}
	if v := os.Getenv("JOB_MAX_ATTEMPTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("JOB_MAX_ATTEMPTS 非法: %q", v)
		}
		cfg.JobMaxAttempts = n
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	return cfg, nil
}
