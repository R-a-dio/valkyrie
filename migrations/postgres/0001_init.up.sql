-- MySQL dump 10.16  Distrib 10.1.32-MariaDB, for Linux (x86_64)
--
-- Host: localhost    Database: radio
-- ------------------------------------------------------
-- Server version	10.1.32-MariaDB

/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!40101 SET NAMES utf8 */;
/*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */;
/*!40103 SET TIME_ZONE='+00:00' */;
/*!40014 SET @OLD_UNIQUE_CHECKS=@@UNIQUE_CHECKS, UNIQUE_CHECKS=0 */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;
/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;

--
-- Table structure for table `djs`
-- 

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `djs` (
  `id` int(12) NOT NULL AUTO_INCREMENT,
  `djname` varchar(60) NOT NULL,
  `djtext` text NOT NULL,
  `djimage` text NOT NULL,
  `visible` int(1) unsigned NOT NULL DEFAULT '0',
  `priority` int(12) unsigned NOT NULL DEFAULT '200',
  `css` varchar(60) NOT NULL DEFAULT '',
  `djcolor` varchar(15) DEFAULT '51 155 185' COMMENT 'RGB values, 0-255 R G B',
  `role` varchar(50) NOT NULL DEFAULT '',
  `theme_id` int(10) unsigned DEFAULT NULL,
  `regex` varchar(200) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `efave`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `efave` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT COMMENT 'db identifier',
  `inick` int(10) unsigned NOT NULL COMMENT 'nick id',
  `isong` int(10) unsigned NOT NULL COMMENT 'song id',
  UNIQUE KEY `inick` (`inick`,`isong`),
  UNIQUE KEY `id` (`id`),
  KEY `isong` (`isong`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `enick`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `enick` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `nick` varchar(30) NOT NULL COMMENT 'irc handle',
  `dta` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'first seen',
  `dtb` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00' COMMENT 'last seen',
  `authcode` varchar(20) DEFAULT NULL,
  `apikey` varchar(256) DEFAULT NULL,
  `type` int(1) unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `nick_2` (`nick`),
  KEY `nick` (`nick`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT='normalized table for irc handles';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `eplay`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `eplay` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT COMMENT 'db identifier',
  `isong` int(10) unsigned NOT NULL COMMENT 'song id',
  `dt` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'datoklokkeslett',
  `ldiff` int(10) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `iplay` (`isong`),
  KEY `eplay_time_index` (`dt`),
  CONSTRAINT `iplay` FOREIGN KEY (`isong`) REFERENCES `esong` (`id`) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT='normalized table for track playback events';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `esong`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `esong` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT COMMENT 'db identifier',
  `hash` varchar(40) NOT NULL COMMENT 'original meta hash',
  `len` int(10) unsigned NOT NULL COMMENT 'seconds',
  `meta` text NOT NULL COMMENT 'current meta',
  `hash_link` varchar(40) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `hash` (`hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT='normalized table for known tracks';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `failed_jobs`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `failed_jobs` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `connection` text NOT NULL,
  `queue` text NOT NULL,
  `payload` text NOT NULL,
  `failed_at` timestamp NULL DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `failed_logins`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `failed_logins` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `ip` varchar(255) NOT NULL,
  `user` varchar(100) NOT NULL,
  `password` varchar(255) NOT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `listenlog`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `listenlog` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `listeners` int(10) unsigned NOT NULL DEFAULT '0',
  `time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `dj` int(11) NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `pending`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `pending` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `artist` varchar(200) NOT NULL,
  `track` varchar(200) NOT NULL,
  `album` varchar(200) NOT NULL,
  `path` text NOT NULL,
  `comment` text NOT NULL,
  `origname` text NOT NULL,
  `submitter` varchar(50) NOT NULL,
  `submitted` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `dupe_flag` int(11) NOT NULL DEFAULT '0',
  `replacement` int(11) DEFAULT NULL,
  `bitrate` int(10) unsigned DEFAULT NULL,
  `length` float DEFAULT '0',
  `format` varchar(10) DEFAULT 'mp3',
  `mode` varchar(10) DEFAULT 'cbr',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `postpending`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `postpending` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `trackid` int(11) DEFAULT NULL,
  `meta` varchar(200) NOT NULL DEFAULT '',
  `ip` varchar(50) NOT NULL DEFAULT '0.0.0.0',
  `accepted` int(1) NOT NULL DEFAULT '0',
  `time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `reason` varchar(120) DEFAULT '',
  `good_upload` int(1) DEFAULT '0',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `queue`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `queue` (
  `trackid` int(14) unsigned NOT NULL,
  `time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `ip` text CHARACTER SET utf8 COLLATE utf8_bin,
  `type` int(3) DEFAULT '0',
  `meta` text,
  `length` float DEFAULT '0',
  `id` int(11) NOT NULL AUTO_INCREMENT,
  PRIMARY KEY (`id`),
  KEY `queue_time_index` (`time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `radio_comments`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `radio_comments` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `comment` varchar(500) NOT NULL,
  `ip` varchar(128) DEFAULT NULL,
  `user_id` int(10) unsigned DEFAULT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT NULL,
  `deleted_at` timestamp NULL DEFAULT NULL,
  `news_id` int(10) unsigned NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `radio_news`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `radio_news` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `title` varchar(200) NOT NULL,
  `text` text,
  `header` text,
  `user_id` int(10) unsigned NOT NULL,
  `deleted_at` timestamp NULL DEFAULT NULL,
  `created_at` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00',
  `updated_at` timestamp NULL DEFAULT NULL,
  `private` tinyint(4) DEFAULT '0',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `requesttime`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `requesttime` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `ip` varchar(50) NOT NULL,
  `time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `streamstatus`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `streamstatus` (
  `id` int(11) NOT NULL DEFAULT '0',
  `djid` int(10) unsigned NOT NULL DEFAULT '0',
  `np` varchar(200) NOT NULL DEFAULT '',
  `listeners` int(10) unsigned NOT NULL DEFAULT '0',
  `bitrate` int(10) unsigned NOT NULL DEFAULT '0',
  `isafkstream` int(1) NOT NULL DEFAULT '0',
  `isstreamdesk` int(1) NOT NULL DEFAULT '0',
  `start_time` bigint(20) unsigned NOT NULL DEFAULT '0',
  `end_time` bigint(20) unsigned NOT NULL DEFAULT '0',
  `lastset` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `trackid` int(12) DEFAULT NULL,
  `thread` text,
  `requesting` int(11) DEFAULT '0',
  `djname` varchar(250) DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `themes`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `themes` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL DEFAULT '',
  `display_name` varchar(255) NOT NULL DEFAULT '',
  `author` varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tracks`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tracks` (
  `id` int(14) unsigned NOT NULL AUTO_INCREMENT,
  `artist` varchar(500) NOT NULL,
  `track` varchar(200) NOT NULL,
  `album` varchar(200) NOT NULL,
  `path` text NOT NULL,
  `tags` text NOT NULL,
  `priority` int(10) NOT NULL DEFAULT '0',
  `lastplayed` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00',
  `lastrequested` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00',
  `usable` int(1) NOT NULL DEFAULT '0',
  `accepter` varchar(200) NOT NULL DEFAULT '',
  `lasteditor` varchar(200) NOT NULL DEFAULT '',
  `hash` varchar(40) DEFAULT NULL,
  `requestcount` int(10) NOT NULL DEFAULT '0',
  `need_reupload` int(1) NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `hash` (`hash`),
  KEY `lastplayedidx` (`lastplayed`),
  KEY `lastrequestedidx` (`lastrequested`),
  FULLTEXT KEY `searchindex` (`tags`,`track`,`artist`,`album`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `uploadtime`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `uploadtime` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `ip` varchar(50) NOT NULL,
  `time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `ip` (`ip`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `users`
--

/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `users` (
  `id` int(12) unsigned NOT NULL AUTO_INCREMENT,
  `user` varchar(50) NOT NULL,
  `pass` varchar(120) DEFAULT NULL,
  `djid` int(12) DEFAULT NULL,
  `privileges` tinyint(3) unsigned NOT NULL DEFAULT '0',
  `updated_at` timestamp NULL DEFAULT NULL,
  `deleted_at` timestamp NULL DEFAULT NULL,
  `created_at` timestamp NULL DEFAULT NULL,
  `email` varchar(255) DEFAULT NULL,
  `remember_token` varchar(100) DEFAULT NULL,
  `ip` varchar(15) DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;

/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;

-- Dump completed on 2019-02-10 14:44:52
