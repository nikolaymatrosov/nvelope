-- Dropping the tables also drops their RLS policies and indexes.
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS recovery_codes;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS user_list_roles;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
