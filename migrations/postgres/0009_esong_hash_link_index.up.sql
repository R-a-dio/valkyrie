-- make all hash_links equal to the actual hash, nothing should be using this column yet so we
-- reset it for consistency sake
UPDATE esong SET esong.hash_link = esong.hash;
-- and then add an index to it
CREATE OR REPLACE INDEX esong_hash_link_index ON esong (hash_link);
