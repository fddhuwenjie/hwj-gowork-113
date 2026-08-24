package httpx

import (
	"net/http"

	"germplasm/internal/service"
)

// handleCreateSensor 注册传感器。
func (s *Server) handleCreateSensor(w http.ResponseWriter, r *http.Request) {
	var in service.CreateSensorInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	sensor, err := s.svc.Sensors.CreateSensor(r.Context(), actorOf(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sensor)
}

// handleListSensors 分页查询传感器。
func (s *Server) handleListSensors(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Sensors.ListSensors(r.Context(), queryParam(r, "chamber"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleAddReading 写入环境读数。
func (s *Server) handleAddReading(w http.ResponseWriter, r *http.Request) {
	var in service.AddReadingInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	rd, err := s.svc.Sensors.AddReading(r.Context(), pathID(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rd)
}

// handleListReadings 分页查询环境读数。
func (s *Server) handleListReadings(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Sensors.ListReadings(r.Context(), queryParam(r, "sensor_id"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleCreateRule 创建保存规则版本。
func (s *Server) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	var in service.CreateRuleInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	rv, err := s.svc.Rules.CreateRuleVersion(r.Context(), actorOf(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rv)
}

// handleListRules 分页查询保存规则。
func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	cursor, limit := pageParams(r)
	page, err := s.svc.Rules.ListRules(r.Context(), queryParam(r, "code"), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleActivateRule 启用保存规则版本。
func (s *Server) handleActivateRule(w http.ResponseWriter, r *http.Request) {
	rv, err := s.svc.Rules.ActivateRule(r.Context(), actorOf(r), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rv)
}
