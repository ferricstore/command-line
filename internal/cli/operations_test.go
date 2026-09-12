package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/buildinfo"
)

type operationsTestClient struct {
	aclSubcommand string
	aclArgs       []any
	leaveCalls    int
}

func (*operationsTestClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (*operationsTestClient) Close() error                                    { return nil }
func (client *operationsTestClient) ACL(_ context.Context, subcommand string, args ...any) (any, error) {
	client.aclSubcommand = subcommand
	client.aclArgs = append([]any(nil), args...)
	return "OK", nil
}
func (client *operationsTestClient) ClusterLeave(context.Context) (bool, error) {
	client.leaveCalls++
	return true, nil
}

func TestACLPasswordStdinBecomesRuleWithoutPrintingSecret(t *testing.T) {
	t.Parallel()

	client := &operationsTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetIn(strings.NewReader("super-secret\n"))
	command.SetArgs([]string{"--profile", "production", "acl", "set-user", "--yes", "--password-stdin", "reporter", "on", "+get"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.aclSubcommand != "SETUSER" {
		t.Fatalf("ACL subcommand = %q", client.aclSubcommand)
	}
	want := []any{"reporter", "on", "+get", ">super-secret"}
	if len(client.aclArgs) != len(want) {
		t.Fatalf("ACL args = %#v", client.aclArgs)
	}
	for index := range want {
		if client.aclArgs[index] != want[index] {
			t.Fatalf("ACL args = %#v, want %#v", client.aclArgs, want)
		}
	}
}

func TestACLSetUserRequiresConfirmationBeforeReadingPasswordOrConnecting(t *testing.T) {
	t.Parallel()

	client := &operationsTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetIn(strings.NewReader("super-secret\n"))
	command.SetArgs([]string{"--profile", "production", "acl", "set-user", "--password-stdin", "reporter", "on", "+get"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.aclSubcommand != "" || len(client.aclArgs) != 0 {
		t.Fatal("ACL SETUSER reached the SDK without confirmation")
	}
}

func TestACLSetUserPreservesNegativeRules(t *testing.T) {
	t.Parallel()

	client := &operationsTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "acl", "set-user", "--yes", "reporter", "reset", "-@all", "+get"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := []any{"reporter", "reset", "-@all", "+get"}
	if len(client.aclArgs) != len(want) {
		t.Fatalf("ACL args = %#v, want %#v", client.aclArgs, want)
	}
	for index := range want {
		if client.aclArgs[index] != want[index] {
			t.Fatalf("ACL args = %#v, want %#v", client.aclArgs, want)
		}
	}
}

func TestClusterMutationRequiresConfirmationBeforeConnecting(t *testing.T) {
	t.Parallel()

	client := &operationsTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "cluster", "leave"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.leaveCalls != 0 {
		t.Fatal("cluster mutation reached SDK without confirmation")
	}
}

func TestClusterMutationRunsAfterConfirmation(t *testing.T) {
	t.Parallel()

	client := &operationsTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "cluster", "leave", "--yes"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.leaveCalls != 1 {
		t.Fatalf("leave calls = %d", client.leaveCalls)
	}
}
