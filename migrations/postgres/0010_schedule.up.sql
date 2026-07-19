CREATE TABLE "schedule" (
  "id" SERIAL,
  "weekday" SMALLINT NOT NULL,
  "text" TEXT NOT NULL,
  "owner" INTEGER,
  "updated_at" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_by" INTEGER NOT NULL,
  "notification" BOOLEAN NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "schedule_owner_user" FOREIGN KEY ("owner") REFERENCES "users" ("id"),
  CONSTRAINT "schedule_updated_by_user" FOREIGN KEY ("updated_by") REFERENCES "users" ("id")
);
CREATE INDEX "updated_at_index" ON "schedule" ("updated_at");
