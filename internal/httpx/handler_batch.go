package httpx

import (
	"net/http"

	"germplasm/internal/service"
)

// handleCreateBatch 建立原始批次。
func (s *Server) handleCreateBatch(w http.ResponseWriter, r *http.Request) {
	var in service.CreateBatchInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	b, err := s.svc.Storage.CreateOriginalBatch(r.Context(), actorOf(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// handleListBatches 分页查询批次。
func (s *Server) handleListBatches(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Storage.ListBatches(r.Context(), queryParam(r, "accession_id"), queryParam(r, "status"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleGetBatch 查询批次详情。
func (s *Server) handleGetBatch(w http.ResponseWriter, r *http.Request) {
	b, err := s.svc.Storage.GetBatch(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// handleSplitSamples 样本分装。
func (s *Server) handleSplitSamples(w http.ResponseWriter, r *http.Request) {
	var in service.SplitSamplesInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.BatchID = pathID(r)
	samples, err := s.svc.Storage.SplitSamples(r.Context(), actorOf(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"items": samples})
}

// handleListSamples 分页查询样本。
func (s *Server) handleListSamples(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Storage.ListSamples(r.Context(), queryParam(r, "batch_id"), queryParam(r, "status"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleGetSample 查询样本详情。
func (s *Server) handleGetSample(w http.ResponseWriter, r *http.Request) {
	smp, err := s.svc.Storage.GetSample(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, smp)
}

// assignLocationBody 库位分配请求体。
type assignLocationBody struct {
	LocationID string `json:"location_id"`
	Version    int64  `json:"version"`
}

// handleAssignLocation 样本分配库位。
func (s *Server) handleAssignLocation(w http.ResponseWriter, r *http.Request) {
	var body assignLocationBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	smp, err := s.svc.Storage.AssignLocation(r.Context(), actorOf(r), pathID(r), body.LocationID, body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, smp)
}

// handleCreateLocation 创建库位。
func (s *Server) handleCreateLocation(w http.ResponseWriter, r *http.Request) {
	var in service.CreateLocationInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	l, err := s.svc.Storage.CreateLocation(r.Context(), actorOf(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

// handleListLocations 分页查询库位。
func (s *Server) handleListLocations(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Storage.ListLocations(r.Context(), queryParam(r, "chamber"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}
