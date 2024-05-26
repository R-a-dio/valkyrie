ALTER TABLE eplay ADD COLUMN IF NOT EXISTS (djs_id INT DEFAULT NULL);

UPDATE eplay INNER JOIN (select time, COALESCE(lead(time) over (order by time asc), NOW()) AS lead, dj from (select time, dj, COALESCE(lag(dj) over (order by time asc), -1) AS prev FROM listenlog ORDER BY time ASC) AS a WHERE prev <> dj) AS log ON eplay.dt BETWEEN log.time AND log.lead SET eplay.djs_id = log.dj;
