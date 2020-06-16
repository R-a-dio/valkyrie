// AUTOMATICALLY GENERATED FILE. DO NOT EDIT.

package migrations

var _ = migration(asset{Name: "0004_relays.up.sql", Content: "" +
	"CREATE TABLE `relays` (\n    `name` varchar(64) NOT NULL,\n    `status` text NOT NULL,\n    `stream` text NOT NULL,\n    `online` boolean NOT NULL DEFAULT 0,\n    `disabled` boolean NOT NULL DEFAULT 0,\n    `noredir` boolean NOT NULL DEFAULT 0,\n    `listeners` int NOT NULL DEFAULT 0,\n    `max` int NOT NULL DEFAULT 0,\n    `err` text NOT NULL DEFAULT \"\",\nPRIMARY KEY (`name`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;" +
	"", etag: `"9CVxJKWcpSQ="`})
