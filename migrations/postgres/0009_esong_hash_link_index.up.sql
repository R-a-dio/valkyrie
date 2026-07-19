-- make all hash_links equal to the actual hash, nothing should be using this column yet so we
-- reset it for consistency sake
UPDATE "esong" SET "hash_link" = "hash";
-- and then add an index to it
CREATE INDEX IF NOT EXISTS "esong_hash_link_index" ON "esong" ("hash_link");
