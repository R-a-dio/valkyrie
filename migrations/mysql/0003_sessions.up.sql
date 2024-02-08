CREATE TABLE `sessions` (
    `id` int(12) unsigned NOT NULL AUTO_INCREMENT,
    `token` varchar(64) NOT NULL,
    `expiry` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00',
    `data` text NOT NULL,
PRIMARY KEY (`id`),
UNIQUE KEY `token` (`token`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;