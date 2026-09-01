ALTER TABLE cancellations
    ADD COLUMN closed_by uuid,
    ADD COLUMN closed_at timestamptz,
    ADD CONSTRAINT cancellations_closed_by_fkey
        FOREIGN KEY (company_id, closed_by) REFERENCES company_users(company_id, user_id) ON DELETE RESTRICT;
UPDATE cancellations SET closed_by=recorded_by,closed_at=updated_at WHERE status='closed';
ALTER TABLE cancellations
    ADD CONSTRAINT cancellations_closed_state_check
        CHECK ((status='closed' AND closed_by IS NOT NULL AND closed_at IS NOT NULL)
            OR (status='recorded' AND closed_by IS NULL AND closed_at IS NULL));

ALTER TABLE return_cases
    ADD COLUMN closed_by uuid,
    ADD COLUMN closed_at timestamptz,
    ADD CONSTRAINT return_cases_closed_by_fkey
        FOREIGN KEY (company_id, closed_by) REFERENCES company_users(company_id, user_id) ON DELETE RESTRICT;
UPDATE return_cases SET closed_by=COALESCE(received_by,created_by),closed_at=updated_at WHERE status='closed';
ALTER TABLE return_cases
    ADD CONSTRAINT return_cases_closed_state_check
        CHECK ((status='closed' AND closed_by IS NOT NULL AND closed_at IS NOT NULL)
            OR (status<>'closed' AND closed_by IS NULL AND closed_at IS NULL));

ALTER TABLE return_events DROP CONSTRAINT return_events_event_type_check;
ALTER TABLE return_events ADD CONSTRAINT return_events_event_type_check
    CHECK (event_type IN ('created','received','inspected','restocked','restock_corrected','closed'));
CREATE INDEX return_events_company_type_created_idx
    ON return_events(company_id, event_type, created_at, id);
CREATE INDEX cancellations_company_cancelled_at_idx
    ON cancellations(company_id, cancelled_at, id);

CREATE TABLE cancellation_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    cancellation_id uuid NOT NULL,
    event_type text NOT NULL CHECK (event_type='closed'),
    actor_user_id uuid NOT NULL,
    notes text CHECK (notes IS NULL OR (notes=btrim(notes) AND notes<>'' AND length(notes)<=2000)),
    idempotency_key text NOT NULL CHECK (idempotency_key=btrim(idempotency_key) AND idempotency_key<>'' AND length(idempotency_key)<=128),
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id, cancellation_id) REFERENCES cancellations(company_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id, actor_user_id) REFERENCES company_users(company_id, user_id) ON DELETE RESTRICT,
    UNIQUE (company_id, id),
    UNIQUE (company_id, idempotency_key)
);
CREATE INDEX cancellation_events_company_cancellation_created_idx
    ON cancellation_events(company_id, cancellation_id, created_at, id);
CREATE INDEX cancellation_events_company_type_created_idx
    ON cancellation_events(company_id, event_type, created_at, id);

CREATE FUNCTION protect_cancellation_events() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'cancellation event history is immutable' USING ERRCODE='55000';
END;
$$;
CREATE TRIGGER cancellation_events_immutable
    BEFORE UPDATE OR DELETE ON cancellation_events
    FOR EACH ROW EXECUTE FUNCTION protect_cancellation_events();
