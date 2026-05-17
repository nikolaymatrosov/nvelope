-- Drop the deferred foreign key first so lists can be dropped while the
-- user_list_roles table (from 000005) still exists.
ALTER TABLE user_list_roles DROP CONSTRAINT IF EXISTS user_list_roles_list_id_fkey;

-- Dropping the tables also drops their RLS policies and indexes.
DROP TABLE IF EXISTS subscriber_lists;
DROP TABLE IF EXISTS subscribers;
DROP TABLE IF EXISTS lists;
