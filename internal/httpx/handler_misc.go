package httpx

import (
	"net/http"

	"germplasm/internal/apperr"
	"germplasm/internal/repository"
)

// handleGetLineage 查询批次谱系。
func (s *Server) handleGetLineage(w http.ResponseWriter, r *http.Request) {
	batchID := queryParam(r, "batch_id")
	if batchID == "" {
		writeError(w, apperr.Validation("缺少查询参数 batch_id"))
		return
	}
	view, err := s.svc.Lineage.GetLineage(r.Context(), batchID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// handleListSnapshots 分页查询历史快照。
func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Repos.Snapshots.List(r.Context(), s.svc.Tx.DB(),
		queryParam(r, "entity_type"), queryParam(r, "entity_id"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleListAlerts 分页查询告警。
func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Repos.Alerts.List(r.Context(), s.svc.Tx.DB(),
		queryParam(r, "status"), queryParam(r, "type"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleAckAlert 确认告警。
func (s *Server) handleAckAlert(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Repos.Alerts.Ack(r.Context(), s.svc.Tx.DB(), pathID(r), s.clk.Now()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ACKED"})
}

// handleListAudit 分页查询审计日志。
func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	entries, next, err := s.svc.Audit.List(r.Context(), s.svc.Tx.DB(),
		queryParam(r, "entity_type"), queryParam(r, "entity_id"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": entries, "next_cursor": next})
}

// handleListJobs 分页查询后台作业。
func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Repos.Jobs.List(r.Context(), s.svc.Tx.DB(), queryParam(r, "status"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleRiskLocations 库位风险巡检。
func (s *Server) handleRiskLocations(w http.ResponseWriter, r *http.Request) {
	risks, err := s.svc.Risk.LocationRisks(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": risks})
}

// handleRiskInventoryVariance 库存差异巡检。
func (s *Server) handleRiskInventoryVariance(w http.ResponseWriter, r *http.Request) {
	vars, err := s.svc.Risk.InventoryVariances(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": vars})
}

// handleRiskGerminationDecline 连续发芽率下降巡检。
func (s *Server) handleRiskGerminationDecline(w http.ResponseWriter, r *http.Request) {
	items, err := s.svc.Risk.GerminationDeclines(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleRiskPendingRestock 待回存批次巡检（分页）。
func (s *Server) handleRiskPendingRestock(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Risk.PendingRestocks(r.Context(), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleRiskLineageAnomalies 谱系异常巡检。
func (s *Server) handleRiskLineageAnomalies(w http.ResponseWriter, r *http.Request) {
	var items []repository.Anomaly
	items, err := s.svc.Lineage.Anomalies(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if items == nil {
		items = []repository.Anomaly{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
