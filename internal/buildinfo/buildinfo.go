package buildinfo

import "runtime/debug"

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func EffectiveVersion() string {
	if Version != "" && Version != "dev" {
		return Version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Version
	}
	if info.Main.Version == "" || info.Main.Version == "(devel)" {
		return Version
	}
	return info.Main.Version
}
