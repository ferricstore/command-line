package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestInfoString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info Info
		want string
	}{
		{
			name: "release",
			info: Info{Version: "v1.2.3", Commit: "abc123", Date: "2026-07-24T12:00:00Z"},
			want: "ferric v1.2.3 (commit abc123, built 2026-07-24T12:00:00Z)",
		},
		{
			name: "development defaults",
			info: Info{},
			want: "ferric dev (commit none, built unknown)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.info.String(); got != test.want {
				t.Fatalf("String() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolveUsesGoInstallModuleAndVCSMetadata(t *testing.T) {
	t.Parallel()

	got := Resolve(Info{Version: "dev", Commit: "none", Date: "unknown"}, &debug.BuildInfo{
		Main: debug.Module{Version: "v0.11.4"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef123456"},
			{Key: "vcs.time", Value: "2026-07-29T10:00:00Z"},
		},
	})
	if got.Version != "v0.11.4" || got.Commit != "abcdef123456" || got.Date != "2026-07-29T10:00:00Z" {
		t.Fatalf("Resolve() = %#v", got)
	}
}

func TestResolveKeepsInjectedReleaseMetadata(t *testing.T) {
	t.Parallel()

	want := Info{Version: "v1.0.0", Commit: "release-commit", Date: "release-date"}
	got := Resolve(want, &debug.BuildInfo{
		Main:     debug.Module{Version: "v0.11.4"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "other"}},
	})
	if got != want {
		t.Fatalf("Resolve() = %#v, want %#v", got, want)
	}
}
