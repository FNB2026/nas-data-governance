CREATE TABLE IF NOT EXISTS dual_purge_plans (
  id TEXT PRIMARY KEY,
  operation TEXT NOT NULL CHECK(operation IN ('QUARANTINE_PURGE','SOURCE_DIRECT_PURGE')),
  subject_id TEXT NOT NULL,
  digest TEXT NOT NULL,
  evidence_json TEXT NOT NULL,
  state TEXT NOT NULL CHECK(state IN ('DRAFT','APPROVED','DRY_RUN','EXECUTING','PURGED','ROLLED_BACK','CANCELLED')),
  expires_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dual_purge_state ON dual_purge_plans(state);
