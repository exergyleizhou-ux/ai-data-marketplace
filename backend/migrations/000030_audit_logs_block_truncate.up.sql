-- The audit_logs "append-only" guarantee was enforced only by a row-level
-- BEFORE UPDATE OR DELETE trigger. In PostgreSQL, row-level triggers NEVER fire
-- on TRUNCATE, so `TRUNCATE audit_logs` silently wiped the whole compliance
-- trail. Add a statement-level BEFORE TRUNCATE trigger reusing the same guard.
--
-- This is defense-in-depth, not a complete fix: the application connects as the
-- table owner, which can still `ALTER TABLE ... DISABLE TRIGGER` / DROP TRIGGER.
-- The full fix is DB privilege separation — run the app as a non-owning role
-- with only INSERT+SELECT on audit_logs (REVOKE TRUNCATE/UPDATE/DELETE/DDL) and
-- run migrations as a separate owner — which is a deployment concern.
CREATE TRIGGER trg_audit_logs_no_truncate
    BEFORE TRUNCATE ON audit_logs
    FOR EACH STATEMENT EXECUTE FUNCTION audit_logs_block_mutation();
