package buildinfo

import "testing"

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
