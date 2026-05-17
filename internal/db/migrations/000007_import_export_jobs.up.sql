-- import_export_jobs tracks asynchronous bulk subscriber import and export
-- work. It is a tenant-plane table protected by Row-Level Security. The staged
-- upload (import) or generated file (export) lives in file_bytes; River's own
-- queue tables, installed by its migrator, carry the durable job state.

CREATE TABLE import_export_jobs (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    kind          text        NOT NULL CHECK (kind IN ('import', 'export')),
    requested_by  uuid        NOT NULL,
    status        text        NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    params        jsonb       NOT NULL DEFAULT '{}',
    file_name     text        NOT NULL DEFAULT '',
    file_bytes    bytea,
    created_count integer     NOT NULL DEFAULT 0,
    updated_count integer     NOT NULL DEFAULT 0,
    failed_count  integer     NOT NULL DEFAULT 0,
    row_count     integer     NOT NULL DEFAULT 0,
    failures      jsonb       NOT NULL DEFAULT '[]',
    created_at    timestamptz NOT NULL DEFAULT now(),
    started_at    timestamptz,
    finished_at   timestamptz
);

ALTER TABLE import_export_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE import_export_jobs FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON import_export_jobs
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON import_export_jobs TO nvelope_app;
