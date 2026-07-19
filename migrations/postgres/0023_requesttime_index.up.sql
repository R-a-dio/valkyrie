ALTER TABLE requesttime MODIFY ip VARCHAR(64);
CREATE INDEX IF NOT EXISTS ip_time ON requesttime (ip, time);