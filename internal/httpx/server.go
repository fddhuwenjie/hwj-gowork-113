package httpx

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"germplasm/internal/clock"
	"germplasm/internal/service"
	"germplasm/internal/store"
)

// Server 聚合 HTTP 服务依赖。
type Server struct {
	svc     *service.Services
	db      *store.DB
	log     *slog.Logger
	clk     clock.Clock
	httpSrv *http.Server
}

// NewServer 创建 HTTP 服务并注册全部路由。
func NewServer(port int, svc *service.Services, db *store.DB, clk clock.Clock, log *slog.Logger) *Server {
	s := &Server{svc: svc, db: db, log: log, clk: clk}
	mux := &router{}
	s.registerRoutes(mux)
	handler := recoverMiddleware(log, loggingMiddleware(log, mux))
	s.httpSrv = &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// Start 启动 HTTP 监听，返回即表示服务进入可用状态。
func (s *Server) Start(errCh chan<- error) {
	go func() {
		s.log.Info("HTTP 服务启动", "addr", s.httpSrv.Addr)
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
}

// Shutdown 优雅关闭 HTTP 服务。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

// handleHealthz 健康检查：数据库可连通即健康。
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "unhealthy", "db": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   clock.Format(s.clk.Now()),
	})
}
