CREATE TABLE "permission_kinds" (
  "permission" VARCHAR(40) NOT NULL,
  PRIMARY KEY ("permission")
);

CREATE TABLE "permissions" (
  "id" SERIAL,
  "user_id" INTEGER NOT NULL,
  "permission" VARCHAR(40) NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "permissions_ibfk_1" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE,
  CONSTRAINT "permissions_ibfk_2" FOREIGN KEY ("permission") REFERENCES "permission_kinds" ("permission") ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX "user_id" ON "permissions" ("user_id");
CREATE INDEX "permission" ON "permissions" ("permission");

INSERT INTO "permission_kinds" ("permission") VALUES
  ('active'),
  ('admin'),
  ('database_delete'),
  ('database_edit'),
  ('database_view'),
  ('dev'),
  ('dj'),
  ('news'),
  ('pending_edit'),
  ('pending_view')
ON CONFLICT ("permission") DO NOTHING;
