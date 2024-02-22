ALTER TABLE eplay ADD COLUMN IF NOT EXISTS (djs_id INT DEFAULT NULL);

UPDATE eplay SET djs_id = (SELECT dj FROM listenlog WHERE eplay.dt >= listenlog.time ORDER BY listenlog.time ASC LIMIT 1);