package httpx

import (
	"net/http"

	"germplasm/internal/service"
)

// handleCreateResource 登记资源。
func (s *Server) handleCreateResource(w http.ResponseWriter, r *http.Request) {
	var in service.CreateResourceInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	res, err := s.svc.Resources.CreateResource(r.Context(), actorOf(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// handleListResources 分页查询资源。
func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Resources.ListResources(r.Context(), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleGetResource 查询资源详情。
func (s *Server) handleGetResource(w http.ResponseWriter, r *http.Request) {
	res, err := s.svc.Resources.GetResource(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleArchiveResource 归档资源。
func (s *Server) handleArchiveResource(w http.ResponseWriter, r *http.Request) {
	var body versionBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	res, err := s.svc.Resources.ArchiveResource(r.Context(), actorOf(r), pathID(r), body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleCreateAccession 登记 accession。
func (s *Server) handleCreateAccession(w http.ResponseWriter, r *http.Request) {
	var in service.CreateAccessionInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	a, err := s.svc.Resources.CreateAccession(r.Context(), actorOf(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// handleListAccessions 分页查询 accession。
func (s *Server) handleListAccessions(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Resources.ListAccessions(r.Context(), queryParam(r, "resource_id"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleGetAccession 查询 accession 详情。
func (s *Server) handleGetAccession(w http.ResponseWriter, r *http.Request) {
	a, err := s.svc.Resources.GetAccession(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}
