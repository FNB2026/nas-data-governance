-- 013_authorization_audit.sql
-- Durable audit stream for business-authorization decisions (R4 Task C,
-- closes residual risk A7's audit half). Every AuthorizationGate decision —
-- eligible or refused — is recorded here before the caller may proceed.
--
-- This table is deliberately separate from operation_logs: operation_logs
-- carries a foreign key into operation_plans, while authorization decisions
-- must also be auditable for plans that do not exist (a probe with a forged
-- plan id is exactly the event worth recording) and for restore/purge plan
-- ids that never appear in operation_plans.
--
-- Content discipline (AGENTS rule 6): opaque identifiers and stable reason
-- codes only — no file paths, no content hashes, no sizes.
CREATE TABLE IF NOT EXISTS authorization_audit (
    id INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL,
    operation TEXT NOT NULL,
    plan_id TEXT NOT NULL,
    item_id TEXT NOT NULL DEFAULT '',
    eligible INTEGER NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '',
    ownership_verified INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_authz_audit_plan ON authorization_audit(plan_id);
CREATE INDEX IF NOT EXISTS idx_authz_audit_time ON authorization_audit(created_at);
