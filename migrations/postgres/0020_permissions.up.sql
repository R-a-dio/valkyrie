-- The permissions foreign key (0002) uses ON UPDATE CASCADE, so the related
-- rows in permissions are updated automatically.
UPDATE "permission_kinds" SET "permission" = 'telemetry_view' WHERE "permission" = 'grafana_view';
