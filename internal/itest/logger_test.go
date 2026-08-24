package itest

import (
	"log/slog"
	"os"
)

// testLogger 返回测试用结构化日志器，仅输出错误级别以减少噪音。
func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
