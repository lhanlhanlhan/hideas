package hideas

import "fmt"

var (
	Version   = "dev"
	BuildTime = "unknown"
)

func localVersionInfo() VersionInfo {
	return VersionInfo{Version: Version, BuildTime: BuildTime}
}

func formatVersionInfo(info VersionInfo) string {
	return fmt.Sprintf("Hideas %s\nbuild time: %s\n", info.Version, info.BuildTime)
}
