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

func TestStoreHelpersAcceptProtocolMinimumArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []any
	}{
		{name: "blmpop", args: []string{"5", "1", "jobs", "LEFT"}, want: []any{"BLMPOP", "5", "1", "jobs", "LEFT"}},
		{name: "xreadgroup", args: []string{"GROUP", "workers", "worker-1", "STREAMS", "events", ">"}, want: []any{"XREADGROUP", "GROUP", "workers", "worker-1", "STREAMS", "events", ">"}},
		{name: "tdigest.merge", args: []string{"latency:all", "1", "latency:a"}, want: []any{"TDIGEST.MERGE", "latency:all", "1", "latency:a"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &cliConnectionClient{result: "OK"}
			command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
			command.SetArgs(append([]string{"--profile", "production", "store", test.name}, test.args...))

			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(client.command, test.want) {
				t.Fatalf("SDK command = %#v, want %#v", client.command, test.want)
			}
		})
	}
}

// Data commands the server gained in 0.12.16 and 0.12.17 (and a few older
// ones) each have a helper that sends the Redis command unchanged.
func TestStoreHelpersCoverServerDataCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "lcs", args: []string{"draft:1", "draft:2", "LEN"}},
		{name: "hgetdel", args: []string{"user:42", "FIELDS", "1", "session"}},
		{name: "hgetex", args: []string{"user:42", "EX", "300", "FIELDS", "1", "session"}},
		{name: "hsetex", args: []string{"user:42", "EX", "300", "FIELDS", "1", "session", "active"}},
		{name: "lmpop", args: []string{"2", "high", "low", "LEFT"}},
		{name: "zunion", args: []string{"2", "scores:a", "scores:b", "WITHSCORES"}},
		{name: "zunionstore", args: []string{"scores:all", "2", "scores:a", "scores:b"}},
		{name: "zinter", args: []string{"2", "scores:a", "scores:b"}},
		{name: "zinterstore", args: []string{"scores:both", "2", "scores:a", "scores:b"}},
		{name: "zintercard", args: []string{"2", "scores:a", "scores:b"}},
		{name: "zdiff", args: []string{"2", "scores:a", "scores:b"}},
		{name: "zdiffstore", args: []string{"scores:only-a", "2", "scores:a", "scores:b"}},
		{name: "zrangestore", args: []string{"top10", "leaderboard", "0", "9"}},
		{name: "zlexcount", args: []string{"names", "[a", "(c"}},
		{name: "zrangebylex", args: []string{"names", "-", "+"}},
		{name: "zrevrangebylex", args: []string{"names", "+", "-"}},
		{name: "zremrangebylex", args: []string{"names", "[a", "(c"}},
		{name: "zremrangebyrank", args: []string{"leaderboard", "0", "-101"}},
		{name: "zremrangebyscore", args: []string{"leaderboard", "-inf", "10"}},
		{name: "zmpop", args: []string{"2", "queue:a", "queue:b", "MIN"}},
		{name: "bzpopmin", args: []string{"queue:a", "5"}},
		{name: "bzpopmax", args: []string{"queue:a", "5"}},
		{name: "bzmpop", args: []string{"5", "1", "queue:a", "MIN"}},
		{name: "xpending", args: []string{"events", "workers"}},
		{name: "xclaim", args: []string{"events", "workers", "worker-2", "60000", "1700000000000-0"}},
		{name: "xautoclaim", args: []string{"events", "workers", "worker-2", "60000", "0-0"}},
		{name: "xsetid", args: []string{"events", "1700000000000-0"}},
		{name: "expiretime", args: []string{"session:42"}},
		{name: "pexpiretime", args: []string{"session:42"}},
		{name: "touch", args: []string{"user:1", "user:2"}},
		{name: "unlink", args: []string{"session:1", "session:2"}},
		{name: "sort", args: []string{"scores"}},
		{name: "sort_ro", args: []string{"scores"}},
		{name: "bitfield", args: []string{"counters", "INCRBY", "u8", "0", "1"}},
		{name: "bitfield_ro", args: []string{"counters", "GET", "u8", "0"}},
		{name: "bf.insert", args: []string{"seen", "ITEMS", "event:1"}},
		{name: "cf.insert", args: []string{"seen", "ITEMS", "event:1"}},
		{name: "cf.insertnx", args: []string{"seen", "ITEMS", "event:1"}},
		{name: "topk.count", args: []string{"popular", "apple"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &cliConnectionClient{result: "OK"}
			command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
			command.SetArgs(append([]string{"--profile", "production", "store", test.name}, test.args...))

			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			want := []any{strings.ToUpper(test.name)}
			for _, argument := range test.args {
				want = append(want, argument)
			}
			if !reflect.DeepEqual(client.command, want) {
				t.Fatalf("SDK command = %#v, want %#v", client.command, want)
			}
		})
	}
}

func TestBlockingStoreHelpersRejectWaitLongerThanCommandTimeoutBeforeConnecting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "blpop", args: []string{"jobs", "30"}},
		{name: "brpop", args: []string{"jobs", "30"}},
		{name: "blmove", args: []string{"pending", "running", "LEFT", "RIGHT", "30"}},
		{name: "blmpop", args: []string{"30", "1", "jobs", "LEFT"}},
		{name: "bzpopmin", args: []string{"queue:a", "30"}},
		{name: "bzpopmax", args: []string{"queue:a", "30"}},
		{name: "bzmpop", args: []string{"30", "1", "queue:a", "MIN"}},
		{name: "xread", args: []string{"BLOCK", "30000", "STREAMS", "events", "0"}},
		{name: "xreadgroup", args: []string{"GROUP", "workers", "worker-1", "BLOCK", "30000", "STREAMS", "events", ">"}},
		{name: "wait", args: []string{"1", "30000"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &cliConnectionClient{}
			command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
			command.SetArgs(append([]string{"--profile", "production", "--timeout", "10s", "store", test.name}, test.args...))

			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), "--timeout") {
				t.Fatalf("Execute() error = %v", err)
			}
			if client.closed || client.command != nil {
				t.Fatal("invalid blocking timeout opened a connection")
			}
		})
	}
}

func TestBlockingWaitParsersRejectDurationOverflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		parser blockingWaitParser
		value  string
	}{
		{name: "seconds", parser: secondsWaitAt(0), value: "1e100"},
		{name: "milliseconds", parser: millisecondsWaitAt(0), value: "9223372036854775807"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := test.parser([]string{test.value}); err == nil {
				t.Fatalf("parser accepted overflowing timeout %q", test.value)
			}
		})
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
