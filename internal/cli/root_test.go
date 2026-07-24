package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/buildinfo"
)

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	command := New(buildinfo.Info{
		Version: "v1.2.3",
		Commit:  "abc123",
		Date:    "2026-07-24T12:00:00Z",
	})
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"version"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := "ferric v1.2.3 (commit abc123, built 2026-07-24T12:00:00Z)"
	if got := strings.TrimSpace(output.String()); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestUnknownCommandReturnsError(t *testing.T) {
	t.Parallel()

	command := New(buildinfo.Info{})
	command.SetArgs([]string{"unknown"})

	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want an unknown command error")
	}
}
