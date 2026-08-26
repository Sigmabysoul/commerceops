ALTER TABLE audit_logs DROP CONSTRAINT IF EXISTS audit_logs_company_actor_fk;
DROP INDEX IF EXISTS sessions_company_user_idx;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_company_user_fk;
ALTER TABLE sessions DROP COLUMN IF EXISTS company_id;
