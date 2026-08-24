package httpx

// registerRoutes 注册全部 HTTP JSON 接口，详见 docs/04-api.md。
func (s *Server) registerRoutes(mux *router) {
	mux.Handle("GET", "/healthz", s.handleHealthz)

	// 资源与 accession
	mux.Handle("POST", "/api/v1/resources", s.handleCreateResource)
	mux.Handle("GET", "/api/v1/resources", s.handleListResources)
	mux.Handle("GET", "/api/v1/resources/{id}", s.handleGetResource)
	mux.Handle("POST", "/api/v1/resources/{id}/archive", s.handleArchiveResource)
	mux.Handle("POST", "/api/v1/accessions", s.handleCreateAccession)
	mux.Handle("GET", "/api/v1/accessions", s.handleListAccessions)
	mux.Handle("GET", "/api/v1/accessions/{id}", s.handleGetAccession)

	// 批次、样本与库位
	mux.Handle("POST", "/api/v1/batches", s.handleCreateBatch)
	mux.Handle("GET", "/api/v1/batches", s.handleListBatches)
	mux.Handle("GET", "/api/v1/batches/{id}", s.handleGetBatch)
	mux.Handle("POST", "/api/v1/batches/{id}/split", s.handleSplitSamples)
	mux.Handle("GET", "/api/v1/samples", s.handleListSamples)
	mux.Handle("GET", "/api/v1/samples/{id}", s.handleGetSample)
	mux.Handle("POST", "/api/v1/samples/{id}/assign-location", s.handleAssignLocation)
	mux.Handle("POST", "/api/v1/locations", s.handleCreateLocation)
	mux.Handle("GET", "/api/v1/locations", s.handleListLocations)

	// 环境监测与保存规则
	mux.Handle("POST", "/api/v1/sensors", s.handleCreateSensor)
	mux.Handle("GET", "/api/v1/sensors", s.handleListSensors)
	mux.Handle("POST", "/api/v1/sensors/{id}/readings", s.handleAddReading)
	mux.Handle("GET", "/api/v1/readings", s.handleListReadings)
	mux.Handle("POST", "/api/v1/rules", s.handleCreateRule)
	mux.Handle("GET", "/api/v1/rules", s.handleListRules)
	mux.Handle("POST", "/api/v1/rules/{id}/activate", s.handleActivateRule)

	// 出库申请
	mux.Handle("POST", "/api/v1/outbound-requests", s.handleCreateOutbound)
	mux.Handle("GET", "/api/v1/outbound-requests", s.handleListOutbound)
	mux.Handle("GET", "/api/v1/outbound-requests/{id}", s.handleGetOutbound)
	mux.Handle("GET", "/api/v1/outbound-requests/{id}/freezes", s.handleListFreezes)
	mux.Handle("POST", "/api/v1/outbound-requests/{id}/approve", s.handleApproveOutbound)
	mux.Handle("POST", "/api/v1/outbound-requests/{id}/reject", s.handleRejectOutbound)
	mux.Handle("POST", "/api/v1/outbound-requests/{id}/fulfill", s.handleFulfillOutbound)
	mux.Handle("POST", "/api/v1/outbound-requests/{id}/cancel", s.handleCancelOutbound)

	// 繁育计划、田间观察与纯度检测
	mux.Handle("POST", "/api/v1/breeding-plans", s.handleCreatePlan)
	mux.Handle("GET", "/api/v1/breeding-plans", s.handleListPlans)
	mux.Handle("GET", "/api/v1/breeding-plans/{id}", s.handleGetPlan)
	mux.Handle("POST", "/api/v1/breeding-plans/{id}/close", s.handleClosePlan)
	mux.Handle("POST", "/api/v1/breeding-plans/{id}/observations", s.handleAddObservation)
	mux.Handle("GET", "/api/v1/breeding-plans/{id}/observations", s.handleListObservations)
	mux.Handle("POST", "/api/v1/purity-tests", s.handleCreateTest)
	mux.Handle("GET", "/api/v1/purity-tests", s.handleListTests)
	mux.Handle("GET", "/api/v1/purity-tests/{id}", s.handleGetTest)
	mux.Handle("POST", "/api/v1/purity-tests/{id}/seal", s.handleSealTest)

	// 回存验收与销毁审批
	mux.Handle("POST", "/api/v1/restock-batches", s.handleCreateRestock)
	mux.Handle("GET", "/api/v1/restock-batches", s.handleListRestock)
	mux.Handle("GET", "/api/v1/restock-batches/{id}", s.handleGetRestock)
	mux.Handle("POST", "/api/v1/restock-batches/{id}/accept", s.handleAcceptRestock)
	mux.Handle("POST", "/api/v1/restock-batches/{id}/reject", s.handleRejectRestock)
	mux.Handle("POST", "/api/v1/destruction-approvals", s.handleCreateDestruction)
	mux.Handle("GET", "/api/v1/destruction-approvals", s.handleListDestruction)
	mux.Handle("POST", "/api/v1/destruction-approvals/{id}/approve", s.handleApproveDestruction)
	mux.Handle("POST", "/api/v1/destruction-approvals/{id}/reject", s.handleRejectDestruction)

	// 谱系、快照、告警、审计与作业
	mux.Handle("GET", "/api/v1/lineage", s.handleGetLineage)
	mux.Handle("GET", "/api/v1/snapshots", s.handleListSnapshots)
	mux.Handle("GET", "/api/v1/alerts", s.handleListAlerts)
	mux.Handle("POST", "/api/v1/alerts/{id}/ack", s.handleAckAlert)
	mux.Handle("GET", "/api/v1/audit-logs", s.handleListAudit)
	mux.Handle("GET", "/api/v1/jobs", s.handleListJobs)

	// 风险巡检
	mux.Handle("GET", "/api/v1/risk/locations", s.handleRiskLocations)
	mux.Handle("GET", "/api/v1/risk/inventory-variance", s.handleRiskInventoryVariance)
	mux.Handle("GET", "/api/v1/risk/germination-decline", s.handleRiskGerminationDecline)
	mux.Handle("GET", "/api/v1/risk/pending-restock", s.handleRiskPendingRestock)
	mux.Handle("GET", "/api/v1/risk/lineage-anomalies", s.handleRiskLineageAnomalies)
}
