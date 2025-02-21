package main

import (
	// search providers
	_ "github.com/R-a-dio/valkyrie/search/bleve"   // search through blevesearch
	_ "github.com/R-a-dio/valkyrie/search/storage" // search through storage provider

	// storage providers
	_ "github.com/R-a-dio/valkyrie/storage/mariadb" // storage through mariadb
)
