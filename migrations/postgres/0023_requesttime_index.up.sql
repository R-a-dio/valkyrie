ALTER TABLE "requesttime" ALTER COLUMN "ip" TYPE VARCHAR(64);
CREATE INDEX IF NOT EXISTS "ip_time" ON "requesttime" ("ip", "time");
