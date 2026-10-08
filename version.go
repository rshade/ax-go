package ax

import (
	"runtime/debug"
	"strconv"
)

const (
	versionUnknown          = "0.0.0-unknown"
	versionDev              = "dev"
	versionBareUnknown      = "unknown"
	buildInfoMainVersionDev = "(devel)"
	buildInfoVCSRevisionKey = "vcs.revision"
	buildInfoVCSModifiedKey = "vcs.modified"
	buildInfoVCSDirtySuffix = "-dirty"
)

// ResolveVersion returns a non-empty tool version for agent-visible surfaces.
//
// Resolution is deterministic for a given binary and uses this precedence:
// first the injected link-time value, then Go build metadata from the running
// binary, then the sentinel "0.0.0-unknown". The result is never empty and is
// never the bare strings "dev" or "unknown"; pass the returned value to
// WithVersion and WithLoggerLabels so __schema.version, ax.Error.version, and
// the logger "version" label agree.
func ResolveVersion(injected string) string {
	// A false ok means build info is unavailable. Drop the pointer here so
	// resolveVersionFrom does not take that bool as a control flag.
	info, ok := debug.ReadBuildInfo()
	if !ok {
		info = nil
	}
	return resolveVersionFrom(injected, info)
}

func resolveVersionFrom(injected string, info *debug.BuildInfo) string {
	if isUsableVersion(injected) {
		return injected
	}
	if info == nil {
		return versionUnknown
	}

	if isUsableVersion(info.Main.Version) {
		return info.Main.Version
	}

	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case buildInfoVCSRevisionKey:
			revision = setting.Value
		case buildInfoVCSModifiedKey:
			modified = setting.Value == strconv.FormatBool(true)
		}
	}
	if !isUsableVersion(revision) {
		return versionUnknown
	}
	if modified {
		return revision + buildInfoVCSDirtySuffix
	}
	return revision
}

func isUsableVersion(version string) bool {
	return version != "" &&
		version != versionDev &&
		version != versionBareUnknown &&
		version != versionUnknown &&
		version != buildInfoMainVersionDev
}
