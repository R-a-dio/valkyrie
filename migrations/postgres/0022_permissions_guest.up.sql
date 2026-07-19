INSERT INTO "permission_kinds" ("permission") VALUES
  ('guest')
ON CONFLICT ("permission") DO NOTHING;
