// AUTOMATICALLY GENERATED FILE. DO NOT EDIT.

package migrations

var _ = migration(asset{Name: "0001_init.up.sql", Content: "" +
	"-- MySQL dump 10.16  Distrib 10.1.32-MariaDB, for Linux (x86_64)\n--\n-- Host: localhost    Database: radio\n-- ------------------------------------------------------\n-- Server version\t10.1.32-MariaDB\n\n/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;\n/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;\n/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;\n/*!40101 SET NAMES utf8 */;\n/*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */;\n/*!40103 SET TIME_ZONE='+00:00' */;\n/*!40014 SET @OLD_UNIQUE_CHECKS=@@UNIQUE_CHECKS, UNIQUE_CHECKS=0 */;\n/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;\n/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;\n/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;\n\n--\n-- Table structure for table `djs`\n-- \n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `djs` (\n  `id` int(12) NOT NULL AUTO_INCREMENT,\n  `djname` varchar(60) NOT NULL,\n  `djtext` text NOT NULL,\n  `djimage` text NOT NULL,\n  `visible` int(1) unsigned NOT NULL DEFAULT '0',\n  `priority` int(12) unsigned NOT NULL DEFAULT '200',\n  `css` varchar(60) NOT NULL DEFAULT '',\n  `djcolor` varchar(15) DEFAULT '51 155 185' COMMENT 'RGB values, 0-255 R G B',\n  `role` varchar(50) NOT NULL DEFAULT '',\n  `theme_id` int(10) unsigned DEFAULT NULL,\n  `regex` varchar(200) NOT NULL DEFAULT '',\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `efave`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `efave` (\n  `id` int(10) unsigned NOT NULL AUTO_INCREMENT COMMENT 'db identifier',\n  `inick` int(10) unsigned NOT NULL COMMENT 'nick id',\n  `isong` int(10) unsigned NOT NULL COMMENT 'song id',\n  UNIQUE KEY `inick` (`inick`,`isong`),\n  UNIQUE KEY `id` (`id`),\n  KEY `isong` (`isong`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `enick`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `enick` (\n  `id` int(10) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',\n  `nick` varchar(30) NOT NULL COMMENT 'irc handle',\n  `dta` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'first seen',\n  `dtb` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00' COMMENT 'last seen',\n  `authcode` varchar(20) DEFAULT NULL,\n  `apikey` varchar(256) DEFAULT NULL,\n  `type` int(1) unsigned DEFAULT NULL,\n  PRIMARY KEY (`id`),\n  UNIQUE KEY `nick_2` (`nick`),\n  KEY `nick` (`nick`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT='normalized table for irc handles';\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `eplay`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `eplay` (\n  `id` int(10) unsigned NOT NULL AUTO_INCREMENT COMMENT 'db identifier',\n  `isong` int(10) unsigned NOT NULL COMMENT 'song id',\n  `dt` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'datoklokkeslett',\n  `ldiff` int(10) DEFAULT NULL,\n  PRIMARY KEY (`id`),\n  KEY `iplay` (`isong`),\n  KEY `eplay_time_index` (`dt`),\n  CONSTRAINT `iplay` FOREIGN KEY (`isong`) REFERENCES `esong` (`id`) ON DELETE CASCADE ON UPDATE CASCADE\n) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT='normalized table for track playback events';\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `esong`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `esong` (\n  `id` int(10) unsigned NOT NULL AUTO_INCREMENT COMMENT 'db identifier',\n  `hash` varchar(40) NOT NULL COMMENT 'original meta hash',\n  `len` int(10) unsigned NOT NULL COMMENT 'seconds',\n  `meta` text NOT NULL COMMENT 'current meta',\n  `hash_link` varchar(40) NOT NULL DEFAULT '',\n  PRIMARY KEY (`id`),\n  UNIQUE KEY `hash` (`hash`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT='normalized table for known tracks';\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `failed_jobs`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `failed_jobs` (\n  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,\n  `connection` text NOT NULL,\n  `queue` text NOT NULL,\n  `payload` text NOT NULL,\n  `failed_at` timestamp NULL DEFAULT NULL,\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `failed_logins`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `failed_logins` (\n  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,\n  `ip` varchar(255) NOT NULL,\n  `user` varchar(100) NOT NULL,\n  `password` varchar(255) NOT NULL,\n  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,\n  `updated_at` timestamp NULL DEFAULT NULL,\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `listenlog`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `listenlog` (\n  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,\n  `listeners` int(10) unsigned NOT NULL DEFAULT '0',\n  `time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,\n  `dj` int(11) NOT NULL DEFAULT '0',\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=latin1;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `pending`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `pending` (\n  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,\n  `artist` varchar(200) NOT NULL,\n  `track` varchar(200) NOT NULL,\n  `album` varchar(200) NOT NULL,\n  `path` text NOT NULL,\n  `comment` text NOT NULL,\n  `origname` text NOT NULL,\n  `submitter` varchar(50) NOT NULL,\n  `submitted` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,\n  `dupe_flag` int(11) NOT NULL DEFAULT '0',\n  `replacement` int(11) DEFAULT NULL,\n  `bitrate` int(10) unsigned DEFAULT NULL,\n  `length` float DEFAULT '0',\n  `format` varchar(10) DEFAULT 'mp3',\n  `mode` varchar(10) DEFAULT 'cbr',\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `postpending`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `postpending` (\n  `id` int(11) NOT NULL AUTO_INCREMENT,\n  `trackid` int(11) DEFAULT NULL,\n  `meta` varchar(200) NOT NULL DEFAULT '',\n  `ip` varchar(50) NOT NULL DEFAULT '0.0.0.0',\n  `accepted` int(1) NOT NULL DEFAULT '0',\n  `time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,\n  `reason` varchar(120) DEFAULT '',\n  `good_upload` int(1) DEFAULT '0',\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `queue`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `queue` (\n  `trackid` int(14) unsigned NOT NULL,\n  `time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,\n  `ip` text CHARACTER SET utf8 COLLATE utf8_bin,\n  `type` int(3) DEFAULT '0',\n  `meta` text,\n  `length` float DEFAULT '0',\n  `id` int(11) NOT NULL AUTO_INCREMENT,\n  PRIMARY KEY (`id`),\n  KEY `queue_time_index` (`time`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `radio_comments`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `radio_comments` (\n  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,\n  `comment` varchar(500) NOT NULL,\n  `ip` varchar(128) DEFAULT NULL,\n  `user_id` int(10) unsigned DEFAULT NULL,\n  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,\n  `updated_at` timestamp NULL DEFAULT NULL,\n  `deleted_at` timestamp NULL DEFAULT NULL,\n  `news_id` int(10) unsigned NOT NULL,\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `radio_news`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `radio_news` (\n  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,\n  `title` varchar(200) NOT NULL,\n  `text` text,\n  `header` text,\n  `user_id` int(10) unsigned NOT NULL,\n  `deleted_at` timestamp NULL DEFAULT NULL,\n  `created_at` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00',\n  `updated_at` timestamp NULL DEFAULT NULL,\n  `private` tinyint(4) DEFAULT '0',\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `requesttime`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `requesttime` (\n  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,\n  `ip` varchar(50) NOT NULL,\n  `time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `streamstatus`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `streamstatus` (\n  `id` int(11) NOT NULL DEFAULT '0',\n  `djid` int(10) unsigned NOT NULL DEFAULT '0',\n  `np` varchar(200) NOT NULL DEFAULT '',\n  `listeners` int(10) unsigned NOT NULL DEFAULT '0',\n  `bitrate` int(10) unsigned NOT NULL DEFAULT '0',\n  `isafkstream` int(1) NOT NULL DEFAULT '0',\n  `isstreamdesk` int(1) NOT NULL DEFAULT '0',\n  `start_time` bigint(20) unsigned NOT NULL DEFAULT '0',\n  `end_time` bigint(20) unsigned NOT NULL DEFAULT '0',\n  `lastset` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,\n  `trackid` int(12) DEFAULT NULL,\n  `thread` text,\n  `requesting` int(11) DEFAULT '0',\n  `djname` varchar(250) DEFAULT '',\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `themes`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `themes` (\n  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,\n  `name` varchar(255) NOT NULL DEFAULT '',\n  `display_name` varchar(255) NOT NULL DEFAULT '',\n  `author` varchar(255) NOT NULL DEFAULT '',\n  PRIMARY KEY (`id`),\n  UNIQUE KEY `name` (`name`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `tracks`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `tracks` (\n  `id` int(14) unsigned NOT NULL AUTO_INCREMENT,\n  `artist` varchar(500) NOT NULL,\n  `track` varchar(200) NOT NULL,\n  `album` varchar(200) NOT NULL,\n  `path` text NOT NULL,\n  `tags` text NOT NULL,\n  `priority` int(10) NOT NULL DEFAULT '0',\n  `lastplayed` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00',\n  `lastrequested` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00',\n  `usable` int(1) NOT NULL DEFAULT '0',\n  `accepter` varchar(200) NOT NULL DEFAULT '',\n  `lasteditor` varchar(200) NOT NULL DEFAULT '',\n  `hash` varchar(40) DEFAULT NULL,\n  `requestcount` int(10) NOT NULL DEFAULT '0',\n  `need_reupload` int(1) NOT NULL DEFAULT '0',\n  PRIMARY KEY (`id`),\n  UNIQUE KEY `hash` (`hash`),\n  KEY `lastplayedidx` (`lastplayed`),\n  KEY `lastrequestedidx` (`lastrequested`),\n  FULLTEXT KEY `searchindex` (`tags`,`track`,`artist`,`album`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `uploadtime`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `uploadtime` (\n  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,\n  `ip` varchar(50) NOT NULL,\n  `time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,\n  PRIMARY KEY (`id`),\n  UNIQUE KEY `ip` (`ip`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n\n--\n-- Table structure for table `users`\n--\n\n/*!40101 SET @saved_cs_client     = @@character_set_client */;\n/*!40101 SET character_set_client = utf8 */;\nCREATE TABLE `users` (\n  `id` int(12) unsigned NOT NULL AUTO_INCREMENT,\n  `user` varchar(50) NOT NULL,\n  `pass` varchar(120) DEFAULT NULL,\n  `djid` int(12) DEFAULT NULL,\n  `privileges` tinyint(3) unsigned NOT NULL DEFAULT '0',\n  `updated_at` timestamp NULL DEFAULT NULL,\n  `deleted_at` timestamp NULL DEFAULT NULL,\n  `created_at` timestamp NULL DEFAULT NULL,\n  `email` varchar(255) DEFAULT NULL,\n  `remember_token` varchar(100) DEFAULT NULL,\n  `ip` varchar(15) DEFAULT '',\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8;\n/*!40101 SET character_set_client = @saved_cs_client */;\n/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;\n\n/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;\n/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;\n/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;\n/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;\n/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;\n/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;\n/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;\n\n-- Dump completed on 2019-02-10 14:44:52\n" +
	"", etag: `"5lD7WZBu9Bs="`})
