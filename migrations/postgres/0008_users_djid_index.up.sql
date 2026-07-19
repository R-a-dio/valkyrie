-- MySQL `CREATE OR REPLACE UNIQUE INDEX` is not supported by PostgreSQL;
-- `CREATE UNIQUE INDEX IF NOT EXISTS` is the idiomatic equivalent.
CREATE UNIQUE INDEX IF NOT EXISTS "users_djid_index" ON "users" ("djid");
