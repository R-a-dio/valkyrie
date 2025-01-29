CREATE TABLE `permission_kinds` (
  `permission` varchar(40) NOT NULL,
  PRIMARY KEY (`permission`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `permissions` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `user_id` int(12) unsigned NOT NULL,
  `permission` varchar(40) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `user_id` (`user_id`),
  KEY `permission` (`permission`),
  CONSTRAINT `permissions_ibfk_1` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE,
  CONSTRAINT `permissions_ibfk_2` FOREIGN KEY (`permission`) REFERENCES `permission_kinds` (`permission`) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

INSERT IGNORE INTO `permission_kinds` (
    `permission`
) VALUES
    ("active"),
    ("admin"),
    ("database_delete"),
    ("database_edit"),
    ("database_view"),
    ("dev"),
    ("dj"),
    ("news"),
    ("pending_edit"),
    ("pending_view");