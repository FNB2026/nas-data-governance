-- Additive record-only governance receipts. No source paths or execution grants.
CREATE TABLE IF NOT EXISTS governance_project (id INTEGER PRIMARY KEY CHECK(id=1), scope TEXT NOT NULL);
INSERT OR IGNORE INTO governance_project(id,scope) VALUES(1,lower(hex(randomblob(16))));
CREATE TABLE IF NOT EXISTS governance_previews (
 token TEXT PRIMARY KEY, scope TEXT NOT NULL, expires_at TEXT NOT NULL,
 payload_json TEXT NOT NULL, preview_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS governance_batches (
 id TEXT PRIMARY KEY, scope TEXT NOT NULL, idempotency_key TEXT NOT NULL UNIQUE,
 request_digest TEXT NOT NULL, preview_token TEXT NOT NULL,
 requested_count INTEGER NOT NULL, state TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS governance_outcomes (
 batch_id TEXT NOT NULL REFERENCES governance_batches(id), group_id TEXT NOT NULL,
 status TEXT NOT NULL, reason_code TEXT NOT NULL, decision_revision INTEGER NOT NULL,
 PRIMARY KEY(batch_id,group_id)
);
CREATE TABLE IF NOT EXISTS governance_decision_audit (
 id INTEGER PRIMARY KEY, batch_id TEXT, group_id TEXT NOT NULL, old_decision TEXT NOT NULL,
 new_decision TEXT NOT NULL, subject_version TEXT NOT NULL, decision_revision INTEGER NOT NULL,
 reason TEXT NOT NULL, source TEXT NOT NULL, result TEXT NOT NULL, reason_code TEXT NOT NULL,
 actor TEXT NOT NULL, created_at TEXT NOT NULL
);
