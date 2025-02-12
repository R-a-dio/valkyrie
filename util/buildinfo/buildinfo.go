package buildinfo

import (
	"runtime/debug"
)

const (
	keyRevision = "vcs.revision"
	keyTime     = "vcs.time"
	keyModified = "vcs.modified"
)

var (
	GitRef                 = getBuildInfoKey(keyRevision, "(devel)")
	GitMod                 = getBuildInfoKey(keyTime, "unknown")
	InstrumentationName    = "github.com/R-a-dio/valkyrie"
	InstrumentationVersion = GitRef
	Version                = GitRef
	ShortRef               = GitRef[:7]
)

func getBuildInfoKey(key string, def string) string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == key {
				return setting.Value
			}
		}
	}
	return def
}
