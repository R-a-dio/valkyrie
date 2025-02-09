package buildinfo

import (
	"runtime/debug"
)

var (
	InstrumentationName    = "github.com/R-a-dio/valkyrie"
	InstrumentationVersion = commitHash()
	Version                = commitHash()
	GitRef                 = commitHash()
	ShortRef               = GitRef[:7]
)

func commitHash() string {
	if info, ok := debug.ReadBuildInfo(); ok { // requires go version 1.12+
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}
	return "(devel)"
}
