// +build !nomanager

package main

import "github.com/R-a-dio/valkyrie/manager"

func init() {
	AddComponent("manager", manager.Component)
}
