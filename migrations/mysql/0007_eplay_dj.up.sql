ALTER TABLE eplay ADD COLUMN IF NOT EXISTS (djs_id INT DEFAULT NULL);

update eplay set djs_id = (SELECT dj FROM listenlog WHERE eplay.dt >= listenlog.time ORDER BY listenlog.time ASC LIMIT 1) where eplay.dt > (SELECT time FROM listenlog ORDER BY time ASC limit 1);