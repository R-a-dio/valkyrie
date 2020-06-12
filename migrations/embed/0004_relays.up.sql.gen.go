// AUTOMATICALLY GENERATED FILE. DO NOT EDIT.

package migrations

var _ = migration(asset{Name: "0004_relays.up.sql", Content: "" +
	"CREATE TABLE `relays` (\r\n    `name` varchar(64) NOT NULL,\r\n    `status` varchar(64) NOT NULL,\r\n    `stream` varchar(64) NOT NULL,\r\n    `online` boolean NOT NULL,\r\n    `disabled` boolean NOT NULL,\r\n    `noredir` boolean NOT NULL,\r\n    `listeners` int NOT NULL,\r\n    `max` int NOT NULL,\r\n    `err` varchar(64) NOT NULL,\r\nPRIMARY KEY (`name`)\r\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;" +
	"", etag: `"hFwxBpciRbY="`})
