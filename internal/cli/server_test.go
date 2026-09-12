package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/buildinfo"
	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	ferricstore "github.com/ferricstore/ferricstore-go"
)

type cliConnectionClient struct {
	message    []string
	command    []any
	result     any
	commandErr error
	response   string
	pingErr    error
	closeErr   error
	closed     bool
}

func (c *cliConnectionClient) Ping(_ context.Context, message ...string) (string, error) {
	c.message = message
	return c.response, c.pingErr
}

func (c *cliConnectionClient) Command(_ context.Context, args ...any) (any, error) {
	c.command = append([]any(nil), args...)
	return c.result, c.commandErr
}

func (c *cliConnectionClient) Close() error {
	c.closed = true
	return c.closeErr
}

type cliConnectionProvider struct {
	method  profile.AuthMethod
	client  connection.Client
	profile profile.Profile
	secret  string
}

func (p *cliConnectionProvider) Method() profile.AuthMethod {
	return p.method
}

func (p *cliConnectionProvider) Open(_ context.Context, storedProfile profile.Profile, secret string) (connection.Client, error) {
	p.profile = storedProfile
	p.secret = secret
	return p.client, nil
}

func newCLIConnectionService(client connection.Client) *connection.Service {
	return connection.NewService(
		&cliProfileStore{values: map[string]profile.Profile{
			"production": {
				Name: "production",
				Authentication: profile.Authentication{
					Method: profile.AuthMethodPassword,
				},
			},
		}},
		&cliCredentialStore{values: map[string]string{"production": "secret"}},
		&cliConnectionProvider{method: profile.AuthMethodPassword, client: client},
	)
}

func TestServerPingUsesSelectedProfileAndClosesClient(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{response: "hello"}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"--profile", "production", "server", "ping", "hello"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if output.String() != "hello\n" {
		t.Fatalf("output = %q", output.String())
	}
	if len(client.message) != 1 || client.message[0] != "hello" {
		t.Fatalf("PING message = %#v", client.message)
	}
	if !client.closed {
		t.Fatal("client was not closed")
	}
}

func TestServerPingClosesClientAfterCommandFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("connection lost")
	client := &cliConnectionClient{pingErr: want}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"server", "ping", "--profile", "production"})

	err := command.Execute()
	if !errors.Is(err, want) {
		t.Fatalf("Execute() error = %v, want PING failure", err)
	}
	if !client.closed {
		t.Fatal("client was not closed after PING failure")
	}
}

func TestNetworkCommandDoesNotFailAcknowledgedOperationWhenCloseFails(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("connection teardown failed")
	client := &cliConnectionClient{result: "OK", closeErr: closeErr}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&diagnostics)
	command.SetArgs([]string{"--profile", "production", "store", "set", "key", "value"})

	if err := command.Execute(); err != nil {
		t.Fatalf("acknowledged SET failed because Close failed: %v", err)
	}
	if output.String() != "OK\n" {
		t.Fatalf("output = %q, want acknowledged result", output.String())
	}
	if !strings.Contains(diagnostics.String(), "warning") || !strings.Contains(diagnostics.String(), closeErr.Error()) {
		t.Fatalf("diagnostics = %q, want close warning", diagnostics.String())
	}
	if !client.closed {
		t.Fatal("client Close was not attempted")
	}
}

func TestServerPingReportsMissingSavedProfile(t *testing.T) {
	t.Parallel()

	command := New(buildinfo.Info{}, WithConnectionService(connection.NewService(
		&cliProfileStore{values: map[string]profile.Profile{}},
		&cliCredentialStore{values: map[string]string{}},
	)))
	command.SetArgs([]string{"server", "ping", "--profile", "missing"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), `load profile "missing"`) {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestServerClientCommandReportsHTTPSAsNativeOnly(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{commandErr: ferricstore.ErrHTTPConnectionAffineCommand}
	stored := profile.Profile{
		Name: "production",
		URL:  "https://store.example.com",
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}
	profiles := &cliProfileStore{values: map[string]profile.Profile{"production": stored}}
	credentials := &cliCredentialStore{values: map[string]string{"production": "secret"}}
	service := connection.NewService(
		profiles,
		credentials,
		&cliConnectionProvider{method: profile.AuthMethodPassword, client: client},
	)
	command := New(buildinfo.Info{}, WithConnectionService(service))
	command.SetArgs([]string{"--profile", "production", "server", "client", "info"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "requires ferric:// or ferrics://") {
		t.Fatalf("Execute() error = %v", err)
	}
	if !client.closed {
		t.Fatal("native-only failure did not close the client")
	}
}

func TestServerKeyInfoUsesSDKCommand(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{result: map[string]any{"type": "string"}}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"--profile", "production", "server", "key-info", "user:42"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := []any{"FERRICSTORE.KEY_INFO", "user:42"}
	if len(client.command) != len(want) {
		t.Fatalf("SDK command = %#v", client.command)
	}
	for index := range want {
		if client.command[index] != want[index] {
			t.Fatalf("SDK command = %#v, want %#v", client.command, want)
		}
	}
	if !strings.Contains(output.String(), `"type": "string"`) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestServerCapabilitiesUsesFerricStoreCapabilitiesCommand(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{result: []any{"GET", "SET"}}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "server", "capabilities"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := []any{"FERRICSTORE.CAPABILITIES"}
	if len(client.command) != len(want) || client.command[0] != want[0] {
		t.Fatalf("SDK command = %#v, want %#v", client.command, want)
	}
}

func TestServerPersistenceMutationRequiresConfirmationBeforeConnecting(t *testing.T) {
	t.Parallel()

	for _, subcommand := range []string{"save", "background-save"} {
		subcommand := subcommand
		t.Run(subcommand, func(t *testing.T) {
			t.Parallel()
			client := &cliConnectionClient{result: "OK"}
			command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
			command.SetArgs([]string{"--profile", "production", "server", "persistence", subcommand})

			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), "requires --yes") {
				t.Fatalf("Execute() error = %v", err)
			}
			if client.command != nil {
				t.Fatalf("persistence mutation reached SDK without confirmation: %#v", client.command)
			}
		})
	}
}

func TestConfirmedSDKCommandPreservesNegativeProtocolValue(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{result: "OK"}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "server", "config", "set", "--yes", "feature.threshold", "-1"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := []any{"CONFIG", "SET", "feature.threshold", "-1"}
	if len(client.command) != len(want) {
		t.Fatalf("SDK command = %#v, want %#v", client.command, want)
	}
	for index := range want {
		if client.command[index] != want[index] {
			t.Fatalf("SDK command = %#v, want %#v", client.command, want)
		}
	}
}
