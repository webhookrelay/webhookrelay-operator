package version

import (
	"runtime/debug"
)

type BuildInfo struct {
	Version   string
	Revision  string
	BuildDate string
}

func GetBuildInfo() *BuildInfo {
	info, _ := debug.ReadBuildInfo()

	bi := &BuildInfo{
		Version: info.Main.Version,
	}

	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			bi.Revision = setting.Value
		}
		if setting.Key == "vcs.time" {
			bi.BuildDate = setting.Value
		}
	}

	return bi
}
