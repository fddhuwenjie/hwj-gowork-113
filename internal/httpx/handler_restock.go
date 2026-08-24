package httpx

import (
	"net/http"

	"germplasm/internal/service"
)

// handleCreateRestock 创建回存验收单。
func (s *Server) handleCreateRestock(w http.ResponseWriter, r *http.Request) {
	var in service.CreateRestockInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	rb, err := s.svc.Restock.Create(r.Context(), actorOf(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rb)
}

// handleListRestock 分页查询回存验收单。
func (s *Server) handleListRestock(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Restock.List(r.Context(), queryParam(r, "status"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleGetRestock 查询回存验收单详情。
func (s *Server) handleGetRestock(w http.ResponseWriter, r *http.Request) {
	rb, err := s.svc.Restock.Get(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rb)
}

// handleAcceptRestock 回存验收通过（创建新批次）。
func (s *Server) handleAcceptRestock(w http.ResponseWriter, r *http.Request) {
	var body versionBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	rb, err := s.svc.Restock.Accept(r.Context(), actorOf(r), pathID(r), body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rb)
}

// rejectRestockBody 驳回回存请求体。
type rejectRestockBody struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}

// handleRejectRestock 驳回回存验收单。
func (s *Server) handleRejectRestock(w http.ResponseWriter, r *http.Request) {
	var body rejectRestockBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	rb, err := s.svc.Restock.Reject(r.Context(), actorOf(r), pathID(r), body.Reason, body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rb)
}

// handleCreateDestruction 提交销毁申请。
func (s *Server) handleCreateDestruction(w http.ResponseWriter, r *http.Request) {
	var in service.CreateDestructionInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	d, err := s.svc.Destruction.Create(r.Context(), actorOf(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

// handleListDestruction 分页查询销毁审批单。
func (s *Server) handleListDestruction(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Destruction.List(r.Context(), queryParam(r, "batch_id"), queryParam(r, "status"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleApproveDestruction 批准销毁。
func (s *Server) handleApproveDestruction(w http.ResponseWriter, r *http.Request) {
	var body versionBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	d, err := s.svc.Destruction.Approve(r.Context(), actorOf(r), pathID(r), body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// handleRejectDestruction 驳回销毁申请。
func (s *Server) handleRejectDestruction(w http.ResponseWriter, r *http.Request) {
	var body versionBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	d, err := s.svc.Destruction.Reject(r.Context(), actorOf(r), pathID(r), body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
