CREATE TABLE `track_metadata` (
    `track_id` int(14) unsigned NOT NULL,
    `id` text(36) NOT NULL,
    `provider` text NOT NULL,
    `album_art_path` text NOT NULL,
    `primary` bit NOT NULL,
    `safe` bit NOT NULL,
PRIMARY KEY (`id`, `provider`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;