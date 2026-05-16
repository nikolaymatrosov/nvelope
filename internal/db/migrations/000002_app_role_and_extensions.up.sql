-- Enable citext for case-insensitive email and slug columns.
CREATE EXTENSION IF NOT EXISTS citext;

-- The restricted runtime role. It is NOT a superuser and NOT BYPASSRLS, so
-- Row-Level Security policies always apply to it — this role is the
-- authoritative tenant-isolation backstop. The dev-default password below MUST
-- be rotated by operations in any non-development environment.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nvelope_app') THEN
        CREATE ROLE nvelope_app LOGIN PASSWORD 'nvelope_app';
    END IF;
END
$$;

GRANT USAGE ON SCHEMA public TO nvelope_app;
