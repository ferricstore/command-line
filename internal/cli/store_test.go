package cli

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/buildinfo"
)

func TestStoreGetUsesSDKAndClosesClient(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{result: "Ada"}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"--profile", "production", "store", "get", "user:42"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(client.command, []any{"GET", "user:42"}) {
		t.Fatalf("SDK command = %#v", client.command)
	}
	if output.String() != "Ada\n" {
		t.Fatalf("output = %q", output.String())
	}
	if !client.closed {
		t.Fatal("client was not closed")
	}
}

func TestStoreRangePreservesNegativeRedisIndex(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{result: []any{"one", "two"}}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "store", "zrange", "leaderboard", "0", "-1", "WITHSCORES"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := []any{"ZRANGE", "leaderboard", "0", "-1", "WITHSCORES"}
	if !reflect.DeepEqual(client.command, want) {
		t.Fatalf("SDK command = %#v, want %#v", client.command, want)
	}
}

func TestStoreRawCommandUsesRequestedWireCommand(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{result: "hello"}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "store", "command", "echo", "hello"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(client.command, []any{"ECHO", "hello"}) {
		t.Fatalf("SDK command = %#v", client.command)
	}
}

func TestStoreRawDestructiveCommandRequiresConfirmationBeforeConnecting(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "store", "command", "FLUSHDB"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.command != nil {
		t.Fatalf("destructive command reached SDK without confirmation: %#v", client.command)
	}
}

func TestStoreRawDestructiveCommandRunsAfterConfirmation(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{result: "OK"}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "store", "command", "FLUSHDB", "--yes"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(client.command, []any{"FLUSHDB"}) {
		t.Fatalf("SDK command = %#v", client.command)
	}
}

func TestStoreRawSafeCommandPreservesTrailingYesArgument(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{result: "--yes"}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "store", "command", "ECHO", "--yes"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(client.command, []any{"ECHO", "--yes"}) {
		t.Fatalf("SDK command = %#v", client.command)
	}
}

func TestStoreRawConfirmedCommandPreservesLiteralYesArgument(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{result: "OK"}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "store", "command", "--yes",
		"CONFIG", "SET", "feature.flag", "--yes",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := []any{"CONFIG", "SET", "feature.flag", "--yes"}
	if !reflect.DeepEqual(client.command, want) {
		t.Fatalf("SDK command = %#v, want %#v", client.command, want)
	}
}

func TestEveryStoreHelperHasDescriptionAndExample(t *testing.T) {
	t.Parallel()

	store := newStoreCommand(dependencies{runtime: &runtimeOptions{timeout: 1}})
	for _, command := range store.Commands() {
		if strings.TrimSpace(command.Short) == "" {
			t.Errorf("%s has no short help", command.Name())
		}
		if strings.TrimSpace(command.Example) == "" {
			t.Errorf("%s has no example", command.Name())
		}
	}
}
