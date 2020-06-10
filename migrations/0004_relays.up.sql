CREATE TABLE `relays` (
    `id` int(12) unsigned NOT NULL AUTO_INCREMENT,
    `name` varchar(64) NOT NULL,
    `status` varchar(64) NOT NULL,
    `stream` varchar(64) NOT NULL,
    `online` boolean NOT NULL,
    `primary` boolean NOT NULL,
    `disabled` boolean NOT NULL,
    `noredir` boolean NOT NULL,
    `listeners` int NOT NULL,
    `max` int NOT NULL,
PRIMARY KEY (`id`),
UNIQUE KEY `name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;