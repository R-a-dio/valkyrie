CREATE TABLE `schedule` (
    `id` int unsigned NOT NULL AUTO_INCREMENT,
    `weekday` TINYINT unsigned NOT NULL,
    `text` TEXT NOT NULL,
    `owner` int unsigned NULL,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_by` int unsigned NOT NULL,
    `notification` BOOLEAN NOT NULL,
    PRIMARY KEY (`id`),
    KEY `updated_at_index` (`updated_at`),
    CONSTRAINT `schedule_owner_user` FOREIGN KEY (`owner`) REFERENCES `users` (`id`),
    CONSTRAINT `schedule_updated_by_user` FOREIGN KEY (`updated_by`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;