// AUTOMATICALLY GENERATED FILE. DO NOT EDIT.

package migrations

var _ = migration(asset{Name: "0003_sessions.up.sql", Content: "" +
	"CREATE TABLE `sessions` (\n    `id` int(12) unsigned NOT NULL AUTO_INCREMENT,\n    `token` varchar(64) NOT NULL,\n    `expiry` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00',\n    `data` text NOT NULL,\nPRIMARY KEY (`id`),\nUNIQUE KEY `token` (`token`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;" +
	"", etag: `"IyIafz7lBY4="`})
