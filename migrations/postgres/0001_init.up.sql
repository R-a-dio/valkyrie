-- PostgreSQL initial schema
-- Converted from MySQL dump (originally MariaDB 10.1.32)
--
-- Host: localhost    Database: radio

--
-- Table structure for table djs
--
CREATE TABLE "djs" (
  "id" SERIAL,
  "djname" VARCHAR(60) NOT NULL,
  "djtext" TEXT NOT NULL,
  "djimage" TEXT NOT NULL,
  "visible" INTEGER NOT NULL DEFAULT 0,
  "priority" INTEGER NOT NULL DEFAULT 200,
  "css" VARCHAR(60) NOT NULL DEFAULT '',
  "djcolor" VARCHAR(15) DEFAULT '51 155 185',
  "role" VARCHAR(50) NOT NULL DEFAULT '',
  "theme_id" INTEGER,
  "regex" VARCHAR(200) NOT NULL DEFAULT '',
  PRIMARY KEY ("id")
);
COMMENT ON COLUMN "djs"."djcolor" IS 'RGB values, 0-255 R G B';

--
-- Table structure for table efave
--
CREATE TABLE "efave" (
  "id" SERIAL,
  "inick" INTEGER NOT NULL,
  "isong" INTEGER NOT NULL,
  UNIQUE ("inick", "isong"),
  UNIQUE ("id")
);
CREATE INDEX "isong" ON "efave" ("isong");
COMMENT ON COLUMN "efave"."id" IS 'db identifier';
COMMENT ON COLUMN "efave"."inick" IS 'nick id';
COMMENT ON COLUMN "efave"."isong" IS 'song id';

--
-- Table structure for table enick
--
CREATE TABLE "enick" (
  "id" SERIAL,
  "nick" VARCHAR(30) NOT NULL,
  "dta" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "dtb" TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00',
  "authcode" VARCHAR(20),
  "apikey" VARCHAR(256),
  "type" SMALLINT,
  PRIMARY KEY ("id"),
  CONSTRAINT "nick_2" UNIQUE ("nick")
);
CREATE INDEX "nick" ON "enick" ("nick");
COMMENT ON COLUMN "enick"."id" IS 'id';
COMMENT ON COLUMN "enick"."nick" IS 'irc handle';
COMMENT ON COLUMN "enick"."dta" IS 'first seen';
COMMENT ON COLUMN "enick"."dtb" IS 'last seen';
COMMENT ON TABLE "enick" IS 'normalized table for irc handles';

--
-- Table structure for table esong
-- (created before eplay because eplay has a foreign key to esong)
--
CREATE TABLE "esong" (
  "id" SERIAL,
  "hash" VARCHAR(40) NOT NULL,
  "len" INTEGER NOT NULL,
  "meta" TEXT NOT NULL,
  "hash_link" VARCHAR(40) NOT NULL DEFAULT '',
  PRIMARY KEY ("id"),
  CONSTRAINT "esong_hash_unique" UNIQUE ("hash")
);
COMMENT ON COLUMN "esong"."id" IS 'db identifier';
COMMENT ON COLUMN "esong"."hash" IS 'original meta hash';
COMMENT ON COLUMN "esong"."len" IS 'seconds';
COMMENT ON COLUMN "esong"."meta" IS 'current meta';
COMMENT ON TABLE "esong" IS 'normalized table for known tracks';

--
-- Table structure for table eplay
--
CREATE TABLE "eplay" (
  "id" SERIAL,
  "isong" INTEGER NOT NULL,
  "dt" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "ldiff" INTEGER,
  PRIMARY KEY ("id"),
  CONSTRAINT "iplay" FOREIGN KEY ("isong") REFERENCES "esong" ("id") ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX "iplay" ON "eplay" ("isong");
CREATE INDEX "eplay_time_index" ON "eplay" ("dt");
COMMENT ON COLUMN "eplay"."id" IS 'db identifier';
COMMENT ON COLUMN "eplay"."isong" IS 'song id';
COMMENT ON COLUMN "eplay"."dt" IS 'datoklokkeslett';
COMMENT ON TABLE "eplay" IS 'normalized table for track playback events';

--
-- Table structure for table failed_jobs
--
CREATE TABLE "failed_jobs" (
  "id" SERIAL,
  "connection" TEXT NOT NULL,
  "queue" TEXT NOT NULL,
  "payload" TEXT NOT NULL,
  "failed_at" TIMESTAMP,
  PRIMARY KEY ("id")
);

--
-- Table structure for table failed_logins
--
CREATE TABLE "failed_logins" (
  "id" SERIAL,
  "ip" VARCHAR(255) NOT NULL,
  "user" VARCHAR(100) NOT NULL,
  "password" VARCHAR(255) NOT NULL,
  "created_at" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" TIMESTAMP,
  PRIMARY KEY ("id")
);

--
-- Table structure for table listenlog
--
CREATE TABLE "listenlog" (
  "id" SERIAL,
  "listeners" INTEGER NOT NULL DEFAULT 0,
  "time" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "dj" INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY ("id")
);

--
-- Table structure for table pending
--
CREATE TABLE "pending" (
  "id" SERIAL,
  "artist" VARCHAR(200) NOT NULL,
  "track" VARCHAR(200) NOT NULL,
  "album" VARCHAR(200) NOT NULL,
  "path" TEXT NOT NULL,
  "comment" TEXT NOT NULL,
  "origname" TEXT NOT NULL,
  "submitter" VARCHAR(50) NOT NULL,
  "submitted" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "dupe_flag" INTEGER NOT NULL DEFAULT 0,
  "replacement" INTEGER,
  "bitrate" INTEGER,
  "length" REAL DEFAULT 0,
  "format" VARCHAR(10) DEFAULT 'mp3',
  "mode" VARCHAR(10) DEFAULT 'cbr',
  PRIMARY KEY ("id")
);

--
-- Table structure for table postpending
--
CREATE TABLE "postpending" (
  "id" SERIAL,
  "trackid" INTEGER,
  "meta" VARCHAR(200) NOT NULL DEFAULT '',
  "ip" VARCHAR(50) NOT NULL DEFAULT '0.0.0.0',
  "accepted" INTEGER NOT NULL DEFAULT 0,
  "time" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "reason" VARCHAR(120) DEFAULT '',
  "good_upload" INTEGER DEFAULT 0,
  PRIMARY KEY ("id")
);

--
-- Table structure for table queue
--
CREATE TABLE "queue" (
  "trackid" INTEGER NOT NULL,
  "time" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "ip" TEXT,
  "type" INTEGER DEFAULT 0,
  "meta" TEXT,
  "length" REAL DEFAULT 0,
  "id" SERIAL,
  PRIMARY KEY ("id")
);
CREATE INDEX "queue_time_index" ON "queue" ("time");

--
-- Table structure for table radio_comments
--
CREATE TABLE "radio_comments" (
  "id" SERIAL,
  "comment" VARCHAR(500) NOT NULL,
  "ip" VARCHAR(128),
  "user_id" INTEGER,
  "created_at" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" TIMESTAMP,
  "deleted_at" TIMESTAMP,
  "news_id" INTEGER NOT NULL,
  PRIMARY KEY ("id")
);

--
-- Table structure for table radio_news
--
CREATE TABLE "radio_news" (
  "id" SERIAL,
  "title" VARCHAR(200) NOT NULL,
  "text" TEXT,
  "header" TEXT,
  "user_id" INTEGER NOT NULL,
  "deleted_at" TIMESTAMP,
  "created_at" TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00',
  "updated_at" TIMESTAMP,
  "private" SMALLINT DEFAULT 0,
  PRIMARY KEY ("id")
);

--
-- Table structure for table requesttime
--
CREATE TABLE "requesttime" (
  "id" SERIAL,
  "ip" VARCHAR(50) NOT NULL,
  "time" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id")
);

--
-- Table structure for table streamstatus
--
CREATE TABLE "streamstatus" (
  "id" INTEGER NOT NULL DEFAULT 0,
  "djid" INTEGER NOT NULL DEFAULT 0,
  "np" VARCHAR(200) NOT NULL DEFAULT '',
  "listeners" INTEGER NOT NULL DEFAULT 0,
  "bitrate" INTEGER NOT NULL DEFAULT 0,
  "isafkstream" INTEGER NOT NULL DEFAULT 0,
  "isstreamdesk" INTEGER NOT NULL DEFAULT 0,
  "start_time" BIGINT NOT NULL DEFAULT 0,
  "end_time" BIGINT NOT NULL DEFAULT 0,
  "lastset" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "trackid" INTEGER,
  "thread" TEXT,
  "requesting" INTEGER DEFAULT 0,
  "djname" VARCHAR(250) DEFAULT '',
  PRIMARY KEY ("id")
);

--
-- Table structure for table themes
--
CREATE TABLE "themes" (
  "id" SERIAL,
  "name" VARCHAR(255) NOT NULL DEFAULT '',
  "display_name" VARCHAR(255) NOT NULL DEFAULT '',
  "author" VARCHAR(255) NOT NULL DEFAULT '',
  PRIMARY KEY ("id"),
  CONSTRAINT "name" UNIQUE ("name")
);

--
-- Table structure for table tracks
--
CREATE TABLE "tracks" (
  "id" SERIAL,
  "artist" VARCHAR(500) NOT NULL,
  "track" VARCHAR(200) NOT NULL,
  "album" VARCHAR(200) NOT NULL,
  "path" TEXT NOT NULL,
  "tags" TEXT NOT NULL,
  "priority" INTEGER NOT NULL DEFAULT 0,
  "lastplayed" TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00',
  "lastrequested" TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00',
  "usable" INTEGER NOT NULL DEFAULT 0,
  "accepter" VARCHAR(200) NOT NULL DEFAULT '',
  "lasteditor" VARCHAR(200) NOT NULL DEFAULT '',
  "hash" VARCHAR(40),
  "requestcount" INTEGER NOT NULL DEFAULT 0,
  "need_reupload" INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY ("id"),
  CONSTRAINT "tracks_hash_unique" UNIQUE ("hash")
);
CREATE INDEX "lastplayedidx" ON "tracks" ("lastplayed");
CREATE INDEX "lastrequestedidx" ON "tracks" ("lastrequested");

--
-- Table structure for table uploadtime
--
CREATE TABLE "uploadtime" (
  "id" SERIAL,
  "ip" VARCHAR(50) NOT NULL,
  "time" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "ip" UNIQUE ("ip")
);

--
-- Table structure for table users
--
CREATE TABLE "users" (
  "id" SERIAL,
  "user" VARCHAR(50) NOT NULL,
  "pass" VARCHAR(120),
  "djid" INTEGER,
  "privileges" SMALLINT NOT NULL DEFAULT 0,
  "updated_at" TIMESTAMP,
  "deleted_at" TIMESTAMP,
  "created_at" TIMESTAMP,
  "email" VARCHAR(255),
  "remember_token" VARCHAR(100),
  "ip" VARCHAR(15) DEFAULT '',
  PRIMARY KEY ("id")
);

--
-- Emulate MySQL's ON UPDATE CURRENT_TIMESTAMP behavior.
-- A single generic trigger function sets the named column (passed as a
-- trigger argument) to CURRENT_TIMESTAMP on every row update.
--
CREATE OR REPLACE FUNCTION set_auto_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW := jsonb_populate_record(NEW, jsonb_build_object(TG_ARGV[0], CURRENT_TIMESTAMP));
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER failed_logins_created_at_auto
  BEFORE UPDATE ON "failed_logins"
  FOR EACH ROW EXECUTE FUNCTION set_auto_timestamp('created_at');

CREATE TRIGGER radio_comments_created_at_auto
  BEFORE UPDATE ON "radio_comments"
  FOR EACH ROW EXECUTE FUNCTION set_auto_timestamp('created_at');

CREATE TRIGGER requesttime_time_auto
  BEFORE UPDATE ON "requesttime"
  FOR EACH ROW EXECUTE FUNCTION set_auto_timestamp('time');

CREATE TRIGGER streamstatus_lastset_auto
  BEFORE UPDATE ON "streamstatus"
  FOR EACH ROW EXECUTE FUNCTION set_auto_timestamp('lastset');

CREATE TRIGGER uploadtime_time_auto
  BEFORE UPDATE ON "uploadtime"
  FOR EACH ROW EXECUTE FUNCTION set_auto_timestamp('time');
