-- Usage metering: the tenant-plane tables behind send metering. usage_events
-- records every billable send; usage_counters is the periodic per-tenant
-- aggregate produced by the usage.rollup job. Both carry tenant_id and the
-- standard tenant_isolation Row-Level-Security policy from 000004.

-- usage_events records one billable action — a campaign or transactional send.
CREATE TABLE usage_events (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    event_type   text        NOT NULL
                             CHECK (event_type IN ('campaign_send', 'transactional_send')),
    quantity     bigint      NOT NULL DEFAULT 1,
    source_ref   text        NOT NULL,
    occurred_at  timestamptz NOT NULL,
    period_start timestamptz NOT NULL,
    rolled_up_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    -- Recording the same send twice is a no-op (research R11).
    UNIQUE (tenant_id, event_type, source_ref)
);

ALTER TABLE usage_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE usage_events FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON usage_events
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

-- The rollup scan and the quota gate's un-rolled tail read both filter to the
-- not-yet-rolled events of a period.
CREATE INDEX usage_events_unrolled_idx
    ON usage_events (tenant_id, period_start) WHERE rolled_up_at IS NULL;

-- usage_counters is a per-tenant, per-period aggregate produced by usage.rollup.
CREATE TABLE usage_counters (
    id                uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    period_start      timestamptz NOT NULL,
    period_end        timestamptz NOT NULL,
    event_type        text        NOT NULL,
    total_quantity    bigint      NOT NULL DEFAULT 0,
    included_quantity bigint      NOT NULL DEFAULT 0,
    overage_quantity  bigint      NOT NULL DEFAULT 0,
    updated_at        timestamptz NOT NULL DEFAULT now(),
    -- One counter per period — usage.rollup upserts this row.
    UNIQUE (tenant_id, period_start, event_type)
);

ALTER TABLE usage_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE usage_counters FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON usage_counters
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON usage_events, usage_counters TO nvelope_app;
