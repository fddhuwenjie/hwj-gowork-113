// Package logging 基于 log/slog 提供结构化 JSON 日志。
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New 依据级别字符串创建输出到 stderr 的 JSON 日志器。
func New(level string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lv = slog.LevelDebug
	case "warn", "warning":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lv})
	return slog.New(h)
}
