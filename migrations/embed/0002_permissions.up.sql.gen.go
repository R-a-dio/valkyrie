// AUTOMATICALLY GENERATED FILE. DO NOT EDIT.

package migrations

var _ = migration(asset{Name: "0002_permissions.up.sql", Content: "" +
	"CREATE TABLE `permission_kinds` (\n  `permission` varchar(40) NOT NULL,\n  PRIMARY KEY (`permission`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n\nCREATE TABLE `permissions` (\n  `id` int(11) NOT NULL AUTO_INCREMENT,\n  `user_id` int(12) unsigned NOT NULL,\n  `permission` varchar(40) NOT NULL,\n  PRIMARY KEY (`id`),\n  KEY `user_id` (`user_id`),\n  KEY `permission` (`permission`),\n  CONSTRAINT `permissions_ibfk_1` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE,\n  CONSTRAINT `permissions_ibfk_2` FOREIGN KEY (`permission`) REFERENCES `permission_kinds` (`permission`) ON DELETE CASCADE ON UPDATE CASCADE\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n\nINSERT IGNORE INTO `permission_kinds` (\n    `permission`\n) VALUES \n    (\"active\"),\n    (\"admin\"),\n    (\"database_delete\"),\n    (\"database_edit\"),\n    (\"database_view\"),\n    (\"dev\"),\n    (\"dj\"),\n    (\"news\"),\n    (\"pending_edit\"),\n    (\"pending_view\");\n" +
	"", etag: `"ulLP1XGcFMI="`})
