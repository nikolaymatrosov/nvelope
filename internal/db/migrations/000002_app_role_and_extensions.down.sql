-- The nvelope_app role is intentionally NOT dropped here: a PostgreSQL role is
-- a cluster-global object, not per-database schema, and may be shared by other
-- databases in the same cluster. A migration reverts schema, not cluster
-- globals. The role is created with IF NOT EXISTS, so re-applying is safe.
DROP EXTENSION IF EXISTS citext;
