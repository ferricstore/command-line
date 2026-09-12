package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/buildinfo"
	"github.com/spf13/cobra"
)

func TestReadTransactionCommandsParsesPortableJSONBatch(t *testing.T) {
	t.Parallel()

	command := &cobra.Command{}
	command.SetIn(strings.NewReader(`[["SET","counter","1"],["INCRBY","counter",2]]`))
	commands, err := readTransactionCommands(command, "-")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]any{{"SET", "counter", "1"}, {"INCRBY", "counter", int64(2)}}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
}

func TestReadTransactionCommandsRejectsEmptyAndMalformedItems(t *testing.T) {
	t.Parallel()

	for _, input := range []string{`[]`, `[[]]`, `[[42,"key"]]`, `not-json`} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			command := &cobra.Command{}
			command.SetIn(strings.NewReader(input))
			if _, err := readTransactionCommands(command, "-"); err == nil {
				t.Fatal("readTransactionCommands() error = nil")
			}
		})
	}
}

func TestTransactionDestructiveCommandRequiresConfirmationBeforeConnecting(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetIn(strings.NewReader(`[["SET","one","1"],["FLUSHDB"]]`))
	command.SetArgs([]string{"--profile", "production", "store", "transaction", "-"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "transaction command 2 requires --yes") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.closed || client.command != nil {
		t.Fatal("destructive transaction opened a connection without confirmation")
	}
}

func TestTransactionResultRejectsWatchConflict(t *testing.T) {
	t.Parallel()

	result, err := checkedTransactionResult(nil, nil, true)
	if err == nil || !strings.Contains(err.Error(), "watched key changed") {
		t.Fatalf("checkedTransactionResult() = %#v, %v; want WATCH conflict", result, err)
	}
}

func TestTransactionResultPreservesExecutionFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("EXEC failed")
	result, err := checkedTransactionResult(nil, want, true)
	if result != nil || !errors.Is(err, want) {
		t.Fatalf("checkedTransactionResult() = %#v, %v; want execution failure", result, err)
	}
}

func TestTransactionRoutingFlagsPreserveCommasInKeys(t *testing.T) {
	t.Parallel()

	command := newStoreTransactionCommand(dependencies{runtime: &runtimeOptions{}})
	for _, name := range []string{"key", "watch"} {
		flag := command.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("--%s flag is missing", name)
		}
		if got := flag.Value.Type(); got != "stringArray" {
			t.Fatalf("--%s type = %q, want stringArray so commas remain part of the key", name, got)
		}
	}
}
