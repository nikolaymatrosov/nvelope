-- Billing: the plan catalog plus the tenant-plane subscription, invoicing, and
-- payment tables. plans is a control-plane catalog with no tenant data, so —
-- like tenants — it carries no RLS. Every other table is tenant-scoped and
-- protected by the standard tenant_isolation policy from 000004.

-- plans is the platform-managed catalog of purchasable offerings. The same
-- catalog for every tenant: no tenant_id, no RLS.
CREATE TABLE plans (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    code                text        NOT NULL UNIQUE,
    name                text        NOT NULL,
    price_minor         bigint      NOT NULL,
    currency            text        NOT NULL CHECK (currency = 'RUB'),
    billing_period      interval    NOT NULL,
    included_sends      bigint      NOT NULL,
    overage_mode        text        NOT NULL CHECK (overage_mode IN ('block', 'meter')),
    overage_price_minor bigint      NOT NULL DEFAULT 0,
    status              text        NOT NULL CHECK (status IN ('draft', 'published', 'archived')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON plans TO nvelope_app;

-- tenant_subscriptions is the billing relationship between a tenant and a plan,
-- an aggregate root carrying the subscription lifecycle state machine.
CREATE TABLE tenant_subscriptions (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    plan_id              uuid        NOT NULL REFERENCES plans (id),
    state                text        NOT NULL
                                     CHECK (state IN ('pending', 'active', 'past_due',
                                                      'suspended', 'canceled')),
    current_period_start timestamptz NOT NULL,
    current_period_end   timestamptz NOT NULL,
    cancel_at_period_end boolean     NOT NULL DEFAULT false,
    canceled_at          timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE tenant_subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_subscriptions FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant_subscriptions
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

-- At most one non-terminal subscription per tenant.
CREATE UNIQUE INDEX tenant_subscriptions_one_active_idx
    ON tenant_subscriptions (tenant_id) WHERE state <> 'canceled';
CREATE INDEX tenant_subscriptions_period_end_idx ON tenant_subscriptions (current_period_end);
CREATE INDEX tenant_subscriptions_state_idx      ON tenant_subscriptions (state);

-- invoices is a bill for one billing period of a subscription.
CREATE TABLE invoices (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    subscription_id uuid        NOT NULL REFERENCES tenant_subscriptions (id) ON DELETE CASCADE,
    period_start    timestamptz NOT NULL,
    period_end      timestamptz NOT NULL,
    total_minor     bigint      NOT NULL,
    currency        text        NOT NULL,
    status          text        NOT NULL
                                CHECK (status IN ('open', 'paid', 'uncollectible', 'void')),
    attempt_count   integer     NOT NULL DEFAULT 0,
    next_attempt_at timestamptz,
    issued_at       timestamptz NOT NULL DEFAULT now(),
    paid_at         timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    -- One invoice per subscription per period — the backbone of renewal
    -- idempotency.
    UNIQUE (subscription_id, period_start)
);

ALTER TABLE invoices ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoices FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON invoices
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX invoices_tenant_idx       ON invoices (tenant_id);
CREATE INDEX invoices_dunning_idx      ON invoices (status, next_attempt_at);
CREATE INDEX invoices_subscription_idx ON invoices (subscription_id);

-- invoice_line_items is a single charge on an invoice.
CREATE TABLE invoice_line_items (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    invoice_id       uuid        NOT NULL REFERENCES invoices (id) ON DELETE CASCADE,
    kind             text        NOT NULL CHECK (kind IN ('subscription', 'overage')),
    description      text        NOT NULL,
    quantity         bigint      NOT NULL,
    unit_price_minor bigint      NOT NULL,
    amount_minor     bigint      NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE invoice_line_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoice_line_items FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON invoice_line_items
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX invoice_line_items_invoice_idx ON invoice_line_items (invoice_id);

-- payment_attempts records one attempt to charge an invoice through the gateway.
CREATE TABLE payment_attempts (
    id                uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    invoice_id        uuid        NOT NULL REFERENCES invoices (id) ON DELETE CASCADE,
    attempt_number    integer     NOT NULL,
    status            text        NOT NULL CHECK (status IN ('succeeded', 'failed')),
    gateway_reference text,
    failure_reason    text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (invoice_id, attempt_number)
);

ALTER TABLE payment_attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE payment_attempts FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON payment_attempts
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX payment_attempts_invoice_idx ON payment_attempts (invoice_id);

GRANT SELECT, INSERT, UPDATE, DELETE
    ON tenant_subscriptions, invoices, invoice_line_items, payment_attempts
    TO nvelope_app;

-- billing_due_subscriptions resolves the subscriptions that need a charge
-- without an app.tenant_id bound, mirroring the Phase 3/4 SECURITY DEFINER
-- tenant-resolution functions. It projects only (tenant_id, subscription_id,
-- reason) and leaks no other row data, so billing.sweep can fan out
-- billing.charge jobs without bypassing RLS in application code.
CREATE FUNCTION billing_due_subscriptions()
    RETURNS TABLE (tenant_id uuid, subscription_id uuid, reason text)
    LANGUAGE sql SECURITY DEFINER STABLE
    SET search_path = public
    AS $$
        SELECT s.tenant_id, s.id, 'renewal'::text
            FROM tenant_subscriptions s
            WHERE s.state = 'active'
              AND s.current_period_end <= now()
        UNION ALL
        SELECT s.tenant_id, s.id, 'dunning'::text
            FROM tenant_subscriptions s
            WHERE s.state = 'past_due'
              AND EXISTS (
                  SELECT 1 FROM invoices i
                  WHERE i.subscription_id = s.id
                    AND i.status = 'open'
                    AND i.next_attempt_at IS NOT NULL
                    AND i.next_attempt_at <= now()
              )
    $$;

REVOKE ALL ON FUNCTION billing_due_subscriptions() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION billing_due_subscriptions() TO nvelope_app;
