// Package buildinfo contains metadata injected into release binaries.
package buildinfo

import "fmt"

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

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
