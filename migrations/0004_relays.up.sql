CREATE TABLE `relays` (
    `name` varchar(64) NOT NULL,
    `status` text NOT NULL,
    `stream` text NOT NULL,
    `online` boolean NOT NULL DEFAULT 0,
    `disabled` boolean NOT NULL DEFAULT 0,
    `noredir` boolean NOT NULL DEFAULT 0,
    `listeners` int NOT NULL DEFAULT 0,
    `max` int NOT NULL DEFAULT 0,
    `err` text NOT NULL DEFAULT "",
PRIMARY KEY (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;