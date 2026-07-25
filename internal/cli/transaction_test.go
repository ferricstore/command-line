package cli

import (
	"reflect"
	"strings"
	"testing"

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
