-- Baseline migration. Enables pgcrypto for gen_random_uuid(), used by the
-- UUID primary keys of every table introduced in later phases.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
