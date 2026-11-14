package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/buildinfo"
)

// Sharded Pub/Sub, LATENCY and cluster feature levels arrived in FerricStore
// 0.12.17; each has a named helper that sends the server command unchanged.
func TestNewerServerCommandHelpersSendServerCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args []string
		want []any
	}{
		{args: []string{"pubsub", "spublish", "orders:{1}", "ready"}, want: []any{"SPUBLISH", "orders:{1}", "ready"}},
		{args: []string{"pubsub", "shard-channels"}, want: []any{"PUBSUB", "SHARDCHANNELS"}},
		{args: []string{"pubsub", "shard-channels", "orders:*"}, want: []any{"PUBSUB", "SHARDCHANNELS", "orders:*"}},
		{args: []string{"pubsub", "shard-subscribers", "orders:{1}"}, want: []any{"PUBSUB", "SHARDNUMSUB", "orders:{1}"}},
		{args: []string{"server", "latency", "latest"}, want: []any{"LATENCY", "LATEST"}},
		{args: []string{"server", "latency", "history", "command"}, want: []any{"LATENCY", "HISTORY", "command"}},
		{args: []string{"server", "latency", "doctor"}, want: []any{"LATENCY", "DOCTOR"}},
		{args: []string{"server", "latency", "graph", "command"}, want: []any{"LATENCY", "GRAPH", "command"}},
		{args: []string{"server", "latency", "histogram", "get", "set"}, want: []any{"LATENCY", "HISTOGRAM", "get", "set"}},
		{args: []string{"server", "latency", "reset", "--yes", "command"}, want: []any{"LATENCY", "RESET", "command"}},
		{args: []string{"cluster", "features"}, want: []any{"CLUSTER.FEATURES"}},
		{args: []string{"cluster", "features", "raise", "12", "--yes"}, want: []any{"CLUSTER.FEATURES.RAISE", "12"}},
	}
	for _, test := range tests {
		test := test
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			client := &cliConnectionClient{result: "OK"}
			command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
			command.SetArgs(append([]string{"--profile", "production"}, test.args...))

			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(client.command, test.want) {
				t.Fatalf("SDK command = %#v, want %#v", client.command, test.want)
			}
		})
	}
}

func TestNewerServerMutationsRequireConfirmationBeforeConnecting(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"server", "latency", "reset"},
		{"cluster", "features", "raise", "12"},
	} {
		args := args
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			client := &cliConnectionClient{}
			command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
			command.SetArgs(append([]string{"--profile", "production"}, args...))

			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), "requires --yes") {
				t.Fatalf("Execute() error = %v", err)
			}
			if client.command != nil {
				t.Fatalf("unconfirmed command reached the SDK: %#v", client.command)
			}
		})
	}
}

func TestClusterFeaturesRaiseRejectsANonPositiveLevel(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "cluster", "features", "raise", "zero", "--yes"})

	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "level") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.command != nil {
		t.Fatalf("invalid level reached the SDK: %#v", client.command)
	}
}

// ACL CAT, GENPASS, LOG and DRYRUN arrived in FerricStore 0.12.17.
func TestNewerACLHelpersSendACLSubcommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args       []string
		subcommand string
		want       []any
	}{
		{args: []string{"acl", "cat"}, subcommand: "CAT", want: []any{}},
		{args: []string{"acl", "cat", "read"}, subcommand: "CAT", want: []any{"read"}},
		{args: []string{"acl", "genpass"}, subcommand: "GENPASS", want: []any{}},
		{args: []string{"acl", "genpass", "128"}, subcommand: "GENPASS", want: []any{"128"}},
		{args: []string{"acl", "log"}, subcommand: "LOG", want: []any{}},
		{args: []string{"acl", "log", "10"}, subcommand: "LOG", want: []any{"10"}},
		{args: []string{"acl", "log-reset", "--yes"}, subcommand: "LOG", want: []any{"RESET"}},
		{args: []string{"acl", "dryrun", "reporter", "get", "report:1"}, subcommand: "DRYRUN", want: []any{"reporter", "get", "report:1"}},
	}
	for _, test := range tests {
		test := test
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			client := &operationsTestClient{}
			command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
			command.SetArgs(append([]string{"--profile", "production"}, test.args...))

			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if client.aclSubcommand != test.subcommand || !reflect.DeepEqual(append([]any{}, client.aclArgs...), test.want) {
				t.Fatalf("ACL %s %#v, want %s %#v", client.aclSubcommand, client.aclArgs, test.subcommand, test.want)
			}
		})
	}
}

func TestACLLogResetRequiresConfirmationBeforeConnecting(t *testing.T) {
	t.Parallel()

	client := &operationsTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "acl", "log-reset"})

	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.aclSubcommand != "" {
		t.Fatalf("unconfirmed ACL LOG RESET reached the SDK")
	}
}
