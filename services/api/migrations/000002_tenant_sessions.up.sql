DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM sessions) THEN
        RAISE EXCEPTION 'cannot bind existing sessions to companies automatically';
    END IF;
END $$;

ALTER TABLE sessions
    ADD COLUMN company_id uuid NOT NULL;

ALTER TABLE sessions
    ADD CONSTRAINT sessions_company_user_fk
    FOREIGN KEY (company_id, user_id)
    REFERENCES company_users (company_id, user_id)
    ON DELETE CASCADE;

CREATE INDEX sessions_company_user_idx
    ON sessions (company_id, user_id, expires_at)
    WHERE revoked_at IS NULL;

ALTER TABLE audit_logs
    ADD CONSTRAINT audit_logs_company_actor_fk
    FOREIGN KEY (company_id, actor_user_id)
    REFERENCES company_users (company_id, user_id)
    ON DELETE RESTRICT;
