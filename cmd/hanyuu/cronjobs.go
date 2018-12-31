package main

import "github.com/R-a-dio/valkyrie/cronjobs"

func init() {
	AddComponent("listenlog", cronjobs.ListenLog)
	AddComponent("requestcount", cronjobs.RequestCount)
}