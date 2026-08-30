DROP INDEX IF EXISTS print_jobs_company_source_idx;
ALTER TABLE print_jobs
    DROP CONSTRAINT IF EXISTS print_jobs_source_print_job_fk,
    DROP CONSTRAINT IF EXISTS print_jobs_reprint_pair_check,
    DROP COLUMN IF EXISTS reprint_reason,
    DROP COLUMN IF EXISTS source_print_job_id;
DROP TABLE IF EXISTS batch_worker_assignments;
DROP TABLE IF EXISTS worker_assignment_rules;
ALTER TABLE employees DROP CONSTRAINT IF EXISTS employees_company_id_id_unique;
