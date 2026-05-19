-- Seed the plan catalog with two published plans so a fresh deployment is
-- immediately subscribable and the quickstart steps are runnable. plans is
-- platform catalog data, so seeding it from a migration is appropriate. The
-- inserts are idempotent on the unique plan code.

INSERT INTO plans
    (code, name, price_minor, currency, billing_period, included_sends,
     overage_mode, overage_price_minor, status)
VALUES
    ('starter', 'Starter', 990000, 'RUB', '1 month', 50000, 'block', 0, 'published'),
    ('growth',  'Growth', 2990000, 'RUB', '1 month', 250000, 'meter', 8, 'published')
ON CONFLICT (code) DO NOTHING;
