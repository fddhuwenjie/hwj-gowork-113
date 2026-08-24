package httpx

import (
	"net/http"

	"germplasm/internal/service"
)

// handleCreatePlan 建立繁育计划。
func (s *Server) handleCreatePlan(w http.ResponseWriter, r *http.Request) {
	var in service.CreatePlanInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	p, err := s.svc.Breeding.CreatePlan(r.Context(), actorOf(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// handleListPlans 分页查询繁育计划。
func (s *Server) handleListPlans(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Breeding.ListPlans(r.Context(), queryParam(r, "status"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleGetPlan 查询繁育计划详情。
func (s *Server) handleGetPlan(w http.ResponseWriter, r *http.Request) {
	p, err := s.svc.Breeding.GetPlan(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// handleClosePlan 关闭繁育计划。
func (s *Server) handleClosePlan(w http.ResponseWriter, r *http.Request) {
	var body versionBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	p, err := s.svc.Breeding.ClosePlan(r.Context(), actorOf(r), pathID(r), body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// handleAddObservation 追加田间观察。
func (s *Server) handleAddObservation(w http.ResponseWriter, r *http.Request) {
	var in service.AddObservationInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	o, err := s.svc.Breeding.AddObservation(r.Context(), actorOf(r), pathID(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

// handleListObservations 查询田间观察记录。
func (s *Server) handleListObservations(w http.ResponseWriter, r *http.Request) {
	obs, err := s.svc.Breeding.ListObservations(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": obs})
}

// handleCreateTest 登记纯度检测。
func (s *Server) handleCreateTest(w http.ResponseWriter, r *http.Request) {
	var in service.CreateTestInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	t, err := s.svc.Purity.CreateTest(r.Context(), actorOf(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// handleListTests 分页查询纯度检测。
func (s *Server) handleListTests(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Purity.ListTests(r.Context(), queryParam(r, "plan_id"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleGetTest 查询纯度检测详情。
func (s *Server) handleGetTest(w http.ResponseWriter, r *http.Request) {
	t, err := s.svc.Purity.GetTest(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// handleSealTest 封存质量判定。
func (s *Server) handleSealTest(w http.ResponseWriter, r *http.Request) {
	var body versionBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	t, err := s.svc.Purity.SealTest(r.Context(), actorOf(r), pathID(r), body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}
