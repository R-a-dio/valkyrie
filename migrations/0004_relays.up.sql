CREATE TABLE `relays` (
    `name` varchar(64) NOT NULL,
    `status` varchar(64) NOT NULL,
    `stream` varchar(64) NOT NULL,
    `online` boolean NOT NULL,
    `disabled` boolean NOT NULL,
    `noredir` boolean NOT NULL,
    `listeners` int NOT NULL,
    `max` int NOT NULL,
    `err` varchar(64) NOT NULL,
PRIMARY KEY (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;