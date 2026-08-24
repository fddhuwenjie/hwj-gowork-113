package httpx

import (
	"log/slog"
	"net/http"
	"time"
)

// actorKey 为请求上下文中操作者的键。
type contextKey string

const actorKey contextKey = "actor"

// actorOf 提取请求操作者，来自 X-Actor 头，缺省为 system。
func actorOf(r *http.Request) string {
	if v := r.Header.Get("X-Actor"); v != "" {
		return v
	}
	return "system"
}

// statusRecorder 记录响应状态码用于访问日志。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// loggingMiddleware 输出结构化访问日志。
func loggingMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Info("http 请求",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"actor", actorOf(r),
		)
	})
}

// recoverMiddleware 捕获 panic 并输出统一错误。
func recoverMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				log.Error("http panic", "path", r.URL.Path, "panic", p)
				writeJSON(w, http.StatusInternalServerError, map[string]any{
					"error": map[string]any{"code": "INTERNAL", "message": "内部错误"},
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
