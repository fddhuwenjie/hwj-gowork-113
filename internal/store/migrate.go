package store

import (
	"context"
	"database/sql"
	"fmt"
)

// schema 为全量建表语句，按版本号递增，重复执行安全（IF NOT EXISTS）。
var schema = []string{
	`CREATE TABLE IF NOT EXISTS resources (
		id TEXT PRIMARY KEY,
		code TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		species TEXT NOT NULL,
		category TEXT NOT NULL,
		status TEXT NOT NULL,
		remark TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS accessions (
		id TEXT PRIMARY KEY,
		resource_id TEXT NOT NULL REFERENCES resources(id),
		accession_no TEXT NOT NULL UNIQUE,
		origin TEXT NOT NULL DEFAULT '',
		donor TEXT NOT NULL DEFAULT '',
		collected_at TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_accessions_resource ON accessions(resource_id, id)`,
	`CREATE TABLE IF NOT EXISTS batches (
		id TEXT PRIMARY KEY,
		accession_id TEXT NOT NULL REFERENCES accessions(id),
		batch_no TEXT NOT NULL UNIQUE,
		kind TEXT NOT NULL,
		mother_batch_id TEXT NOT NULL DEFAULT '',
		unit TEXT NOT NULL DEFAULT '粒',
		qty_total INTEGER NOT NULL,
		qty_available INTEGER NOT NULL,
		qty_frozen INTEGER NOT NULL DEFAULT 0,
		qty_outbound INTEGER NOT NULL DEFAULT 0,
		qty_destroyed INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		closed_at TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_batches_accession ON batches(accession_id, id)`,
	`CREATE TABLE IF NOT EXISTS samples (
		id TEXT PRIMARY KEY,
		batch_id TEXT NOT NULL REFERENCES batches(id),
		sample_no TEXT NOT NULL UNIQUE,
		qty INTEGER NOT NULL,
		status TEXT NOT NULL,
		location_id TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_samples_batch ON samples(batch_id, status, created_at)`,
	`CREATE TABLE IF NOT EXISTS locations (
		id TEXT PRIMARY KEY,
		code TEXT NOT NULL UNIQUE,
		chamber TEXT NOT NULL,
		rack TEXT NOT NULL,
		box TEXT NOT NULL,
		slot TEXT NOT NULL,
		capacity INTEGER NOT NULL,
		occupied INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_locations_chamber ON locations(chamber, id)`,
	`CREATE TABLE IF NOT EXISTS sensors (
		id TEXT PRIMARY KEY,
		code TEXT NOT NULL UNIQUE,
		chamber TEXT NOT NULL,
		metric TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_sensors_chamber ON sensors(chamber, metric)`,
	`CREATE TABLE IF NOT EXISTS sensor_readings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sensor_id TEXT NOT NULL REFERENCES sensors(id),
		metric TEXT NOT NULL,
		value REAL NOT NULL,
		recorded_at TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_readings_sensor ON sensor_readings(sensor_id, recorded_at)`,
	`CREATE TABLE IF NOT EXISTS rule_versions (
		id TEXT PRIMARY KEY,
		code TEXT NOT NULL,
		version_no INTEGER NOT NULL,
		min_temp REAL NOT NULL,
		max_temp REAL NOT NULL,
		min_humidity REAL NOT NULL,
		max_humidity REAL NOT NULL,
		window_before_hours INTEGER NOT NULL,
		window_after_hours INTEGER NOT NULL,
		min_coverage REAL NOT NULL,
		min_purity REAL NOT NULL,
		status TEXT NOT NULL,
		effective_from TEXT,
		created_at TEXT NOT NULL,
		UNIQUE(code, version_no)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_rules_status ON rule_versions(code, status)`,
	`CREATE TABLE IF NOT EXISTS outbound_requests (
		id TEXT PRIMARY KEY,
		request_no TEXT NOT NULL UNIQUE,
		accession_id TEXT NOT NULL REFERENCES accessions(id),
		batch_id TEXT NOT NULL REFERENCES batches(id),
		qty INTEGER NOT NULL,
		purpose TEXT NOT NULL DEFAULT '',
		breeding_target TEXT NOT NULL DEFAULT '',
		rule_version_id TEXT NOT NULL REFERENCES rule_versions(id),
		deadline TEXT NOT NULL,
		status TEXT NOT NULL,
		idempotency_key TEXT UNIQUE,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_outbound_status ON outbound_requests(status, deadline)`,
	`CREATE TABLE IF NOT EXISTS outbound_freezes (
		id TEXT PRIMARY KEY,
		request_id TEXT NOT NULL REFERENCES outbound_requests(id),
		sample_id TEXT NOT NULL REFERENCES samples(id),
		location_id TEXT NOT NULL DEFAULT '',
		qty INTEGER NOT NULL,
		status TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_freezes_request ON outbound_freezes(request_id, status)`,
	`CREATE TABLE IF NOT EXISTS breeding_plans (
		id TEXT PRIMARY KEY,
		plan_no TEXT NOT NULL UNIQUE,
		outbound_request_id TEXT NOT NULL UNIQUE REFERENCES outbound_requests(id),
		batch_id TEXT NOT NULL REFERENCES batches(id),
		target_qty INTEGER NOT NULL,
		plot TEXT NOT NULL DEFAULT '',
		deadline TEXT NOT NULL,
		status TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_plans_status ON breeding_plans(status, deadline)`,
	`CREATE TABLE IF NOT EXISTS field_observations (
		id TEXT PRIMARY KEY,
		plan_id TEXT NOT NULL REFERENCES breeding_plans(id),
		observed_at TEXT NOT NULL,
		germination_rate REAL NOT NULL,
		vigor TEXT NOT NULL DEFAULT '',
		notes TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_observations_plan ON field_observations(plan_id, observed_at)`,
	`CREATE TABLE IF NOT EXISTS purity_tests (
		id TEXT PRIMARY KEY,
		plan_id TEXT NOT NULL REFERENCES breeding_plans(id),
		sample_qty INTEGER NOT NULL,
		coverage_ratio REAL NOT NULL,
		purity_rate REAL NOT NULL,
		verdict TEXT NOT NULL,
		sealed INTEGER NOT NULL DEFAULT 0,
		sealed_at TEXT,
		tested_at TEXT NOT NULL,
		idempotency_key TEXT UNIQUE,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tests_plan ON purity_tests(plan_id, sealed)`,
	`CREATE TABLE IF NOT EXISTS restock_batches (
		id TEXT PRIMARY KEY,
		request_no TEXT NOT NULL UNIQUE,
		plan_id TEXT NOT NULL REFERENCES breeding_plans(id),
		qty INTEGER NOT NULL,
		status TEXT NOT NULL,
		new_batch_id TEXT NOT NULL DEFAULT '',
		reject_reason TEXT NOT NULL DEFAULT '',
		idempotency_key TEXT UNIQUE,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_restock_status ON restock_batches(status, created_at)`,
	`CREATE TABLE IF NOT EXISTS destruction_approvals (
		id TEXT PRIMARY KEY,
		batch_id TEXT NOT NULL REFERENCES batches(id),
		qty INTEGER NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		approver TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS lineage_edges (
		id TEXT PRIMARY KEY,
		resource_id TEXT NOT NULL REFERENCES resources(id),
		parent_batch_id TEXT NOT NULL REFERENCES batches(id),
		child_batch_id TEXT NOT NULL REFERENCES batches(id),
		relation TEXT NOT NULL,
		created_at TEXT NOT NULL,
		UNIQUE(parent_batch_id, child_batch_id, relation)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_lineage_child ON lineage_edges(child_batch_id)`,
	`CREATE INDEX IF NOT EXISTS idx_lineage_parent ON lineage_edges(parent_batch_id)`,
	`CREATE TABLE IF NOT EXISTS snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		event TEXT NOT NULL,
		payload TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_snapshots_entity ON snapshots(entity_type, entity_id, id)`,
	`CREATE TABLE IF NOT EXISTS idempotency_keys (
		key TEXT NOT NULL,
		endpoint TEXT NOT NULL,
		request_hash TEXT NOT NULL,
		response TEXT NOT NULL,
		status_code INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY(key, endpoint)
	)`,
	`CREATE TABLE IF NOT EXISTS jobs (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		payload TEXT NOT NULL DEFAULT '{}',
		status TEXT NOT NULL,
		attempts INTEGER NOT NULL DEFAULT 0,
		max_attempts INTEGER NOT NULL DEFAULT 5,
		next_run_at TEXT NOT NULL,
		last_error TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_jobs_poll ON jobs(status, next_run_at)`,
	`CREATE TABLE IF NOT EXISTS alerts (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		ref_type TEXT NOT NULL,
		ref_id TEXT NOT NULL,
		message TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at TEXT NOT NULL,
		acked_at TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_alerts_open ON alerts(status, type, ref_type, ref_id)`,
	`CREATE TABLE IF NOT EXISTS audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		actor TEXT NOT NULL,
		action TEXT NOT NULL,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		detail TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_logs(entity_type, entity_id, id)`,
}

// Migrate 在启动时执行全量迁移，保证表结构存在。
func Migrate(ctx context.Context, db *sql.DB) error {
	for i, stmt := range schema {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("执行第 %d 条迁移失败: %w", i+1, err)
		}
	}
	return nil
}
