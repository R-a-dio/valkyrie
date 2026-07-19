ALTER TABLE "eplay" ADD COLUMN IF NOT EXISTS "djs_id" INTEGER DEFAULT NULL;

-- MySQL `UPDATE ... INNER JOIN ... SET ...` rewritten to PostgreSQL
-- `UPDATE ... SET ... FROM ... WHERE ...`. The alias `lead` was renamed to
-- `lead_time` to avoid clashing with the lead() window function.
UPDATE "eplay"
SET "djs_id" = "log"."dj"
FROM (
  SELECT "time",
         COALESCE(lead("time") OVER (ORDER BY "time" ASC), NOW()) AS lead_time,
         "dj"
  FROM (
    SELECT "time", "dj", COALESCE(lag("dj") OVER (ORDER BY "time" ASC), -1) AS prev
    FROM "listenlog"
  ) AS a
  WHERE prev <> "dj"
) AS log
WHERE "eplay"."dt" BETWEEN "log"."time" AND "log"."lead_time";
