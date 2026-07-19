INSERT INTO "permission_kinds" ("permission") VALUES
  ('staff')
ON CONFLICT ("permission") DO NOTHING;
