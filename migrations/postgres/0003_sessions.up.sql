CREATE TABLE "sessions" (
  "id" SERIAL,
  "token" VARCHAR(64) NOT NULL,
  "expiry" TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00',
  "data" TEXT NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "token" UNIQUE ("token")
);
