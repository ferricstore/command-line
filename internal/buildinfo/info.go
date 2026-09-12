// Package buildinfo contains metadata injected into release binaries.
package buildinfo

import (
	"fmt"
	"runtime/debug"
)

// Info describes a CLI build.
type Info struct {
	Version string
	Commit  string
	Date    string
}

// String returns a human-readable build description.
func (i Info) String() string {
	version := valueOr(i.Version, "dev")
	commit := valueOr(i.Commit, "none")
	date := valueOr(i.Date, "unknown")

	return fmt.Sprintf("ferric %s (commit %s, built %s)", version, commit, date)
}

// Resolve fills linker-default metadata from Go module and VCS build data.
// This keeps `go install module@version` binaries identifiable without
// overriding metadata injected by the release pipeline.
func Resolve(injected Info, runtimeInfo *debug.BuildInfo) Info {
	resolved := injected
	if runtimeInfo == nil {
		return resolved
	}
	if (resolved.Version == "" || resolved.Version == "dev") &&
		runtimeInfo.Main.Version != "" && runtimeInfo.Main.Version != "(devel)" {
		resolved.Version = runtimeInfo.Main.Version
	}
	settings := make(map[string]string, len(runtimeInfo.Settings))
	for _, setting := range runtimeInfo.Settings {
		settings[setting.Key] = setting.Value
	}
	if resolved.Commit == "" || resolved.Commit == "none" {
		resolved.Commit = settings["vcs.revision"]
	}
	if resolved.Date == "" || resolved.Date == "unknown" {
		resolved.Date = settings["vcs.time"]
	}
	return resolved
}

// ResolveRuntime reads metadata embedded by the Go toolchain.
func ResolveRuntime(injected Info) Info {
	runtimeInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return injected
	}
	return Resolve(injected, runtimeInfo)
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
