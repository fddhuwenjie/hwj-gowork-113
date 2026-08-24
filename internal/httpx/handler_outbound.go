package httpx

import (
	"net/http"

	"germplasm/internal/service"
)

// handleCreateOutbound 创建出库申请。
func (s *Server) handleCreateOutbound(w http.ResponseWriter, r *http.Request) {
	var in service.CreateOutboundInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	o, err := s.svc.Outbound.Create(r.Context(), actorOf(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

// handleListOutbound 分页查询出库申请。
func (s *Server) handleListOutbound(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Outbound.List(r.Context(), queryParam(r, "status"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleGetOutbound 查询出库申请详情。
func (s *Server) handleGetOutbound(w http.ResponseWriter, r *http.Request) {
	o, err := s.svc.Outbound.Get(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// handleListFreezes 查询出库申请冻结明细。
func (s *Server) handleListFreezes(w http.ResponseWriter, r *http.Request) {
	freezes, err := s.svc.Outbound.ListFreezes(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": freezes})
}

// handleApproveOutbound 审批出库申请（冻结样本/库位/规则/繁育目标）。
func (s *Server) handleApproveOutbound(w http.ResponseWriter, r *http.Request) {
	var body versionBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	o, err := s.svc.Outbound.Approve(r.Context(), actorOf(r), pathID(r), body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// handleRejectOutbound 驳回出库申请。
func (s *Server) handleRejectOutbound(w http.ResponseWriter, r *http.Request) {
	var body versionBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	o, err := s.svc.Outbound.Reject(r.Context(), actorOf(r), pathID(r), body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// handleFulfillOutbound 执行出库。
func (s *Server) handleFulfillOutbound(w http.ResponseWriter, r *http.Request) {
	var body versionBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	o, err := s.svc.Outbound.Fulfill(r.Context(), actorOf(r), pathID(r), body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// handleCancelOutbound 取消出库申请并释放冻结。
func (s *Server) handleCancelOutbound(w http.ResponseWriter, r *http.Request) {
	var body versionBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	o, err := s.svc.Outbound.Cancel(r.Context(), actorOf(r), pathID(r), body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}
