package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/auth"
	"github.com/ferricstore/command-line/internal/buildinfo"
	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
)

type staticCredentialSource struct {
	credentials connection.EphemeralCredentials
	present     bool
	err         error
	calls       int
}

func (s *staticCredentialSource) Configured() bool {
	return s.present
}

func (s *staticCredentialSource) Resolve(context.Context) (connection.EphemeralCredentials, bool, error) {
	s.calls++
	return s.credentials, s.present, s.err
}

func environmentPasswordCredentials() connection.EphemeralCredentials {
	return connection.EphemeralCredentials{
		Profile: profile.Profile{
			URL: "ferrics://environment.example.com:6388",
			Authentication: profile.Authentication{
				Method:   profile.AuthMethodPassword,
				Username: "environment-user",
			},
		},
		Secret: "environment-secret",
	}
}

func TestNetworkCommandPrefersEnvironmentCredentialsOverSelectedProfile(t *testing.T) {
	t.Parallel()

	profiles := &cliProfileStore{values: map[string]profile.Profile{
		"production": {
			Name: "production",
			URL:  "ferric://saved.example.com:6388",
			Authentication: profile.Authentication{
				Method:   profile.AuthMethodPassword,
				Username: "saved-user",
			},
		},
	}}
	credentials := &cliCredentialStore{values: map[string]string{"production": "saved-secret"}}
	provider := &cliConnectionProvider{method: profile.AuthMethodPassword, client: &cliConnectionClient{response: "PONG"}}
	source := &staticCredentialSource{credentials: environmentPasswordCredentials(), present: true}
	command := New(
		buildinfo.Info{},
		WithCredentialStore(credentials),
		WithConnectionService(connection.NewService(profiles, credentials, provider)),
		WithEnvironmentCredentialSource(source),
	)
	command.SetArgs([]string{"server", "ping"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if provider.profile != source.credentials.Profile || provider.secret != source.credentials.Secret {
		t.Fatalf("provider received %#v/%q", provider.profile, provider.secret)
	}
	if source.calls != 1 {
		t.Fatalf("environment source calls = %d", source.calls)
	}
}

func TestDefaultEnvironmentCredentialSourceReadsProcessEnvironment(t *testing.T) {
	t.Setenv("FERRIC_URL", "ferric://environment.example.com:6388")
	t.Setenv("FERRIC_USERNAME", "operator")
	t.Setenv("FERRIC_PASSWORD", "environment-secret")
	t.Setenv("FERRIC_PROFILE", "missing-saved-profile")

	provider := &cliConnectionProvider{
		method: profile.AuthMethodPassword,
		client: &cliConnectionClient{response: "PONG"},
	}
	command := New(
		buildinfo.Info{},
		WithConnectionService(connection.NewService(nil, nil, provider)),
	)
	command.SetArgs([]string{"server", "ping"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if provider.profile.URL != "ferric://environment.example.com:6388" ||
		provider.profile.Authentication.Username != "operator" ||
		provider.secret != "environment-secret" {
		t.Fatalf("provider received %#v/%q", provider.profile, provider.secret)
	}
}

func TestNetworkCommandRoutesEnvironmentAPITokenToEnterpriseProviderMock(t *testing.T) {
	t.Parallel()

	credentials := connection.EphemeralCredentials{
		Profile: profile.Profile{
			ControlURL:   "https://control.example.com",
			Organization: "acme",
			Cluster:      "production",
			Authentication: profile.Authentication{
				Method: profile.AuthMethodEnterpriseAPIToken,
			},
		},
		Secret: "machine-token",
	}
	source := &staticCredentialSource{credentials: credentials, present: true}
	provider := &cliConnectionProvider{
		method: profile.AuthMethodEnterpriseAPIToken,
		client: &cliConnectionClient{response: "PONG"},
	}
	command := New(
		buildinfo.Info{},
		WithConnectionService(connection.NewService(nil, nil, provider)),
		WithEnvironmentCredentialSource(source),
	)
	command.SetArgs([]string{"server", "ping"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if provider.profile != credentials.Profile || provider.secret != credentials.Secret {
		t.Fatalf("Enterprise provider received %#v/%q", provider.profile, provider.secret)
	}
}

func TestExplicitProfileOverridesEnvironmentCredentials(t *testing.T) {
	t.Parallel()

	profiles := &cliProfileStore{values: map[string]profile.Profile{
		"production": {
			Name: "production",
			Authentication: profile.Authentication{
				Method: profile.AuthMethodPassword,
			},
		},
	}}
	credentials := &cliCredentialStore{values: map[string]string{"production": "saved-secret"}}
	provider := &cliConnectionProvider{method: profile.AuthMethodPassword, client: &cliConnectionClient{response: "PONG"}}
	source := &staticCredentialSource{credentials: environmentPasswordCredentials(), present: true}
	command := New(
		buildinfo.Info{},
		WithCredentialStore(credentials),
		WithConnectionService(connection.NewService(profiles, credentials, provider)),
		WithEnvironmentCredentialSource(source),
	)
	command.SetArgs([]string{"--profile", "production", "server", "ping"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if provider.profile.Name != "production" || provider.secret != "saved-secret" {
		t.Fatalf("provider received %#v/%q", provider.profile, provider.secret)
	}
	if source.calls != 0 {
		t.Fatalf("explicit profile still resolved environment credentials %d time(s)", source.calls)
	}
}

func TestInvalidEnvironmentCredentialsFailBeforeConnecting(t *testing.T) {
	t.Parallel()

	want := errors.New("FERRIC_USERNAME is required")
	source := &staticCredentialSource{present: true, err: want}
	provider := &cliConnectionProvider{method: profile.AuthMethodPassword, client: &cliConnectionClient{response: "PONG"}}
	command := New(
		buildinfo.Info{},
		WithConnectionService(connection.NewService(nil, nil, provider)),
		WithEnvironmentCredentialSource(source),
	)
	command.SetArgs([]string{"server", "ping"})

	if err := command.Execute(); !errors.Is(err, want) {
		t.Fatalf("Execute() error = %v", err)
	}
	if provider.profile != (profile.Profile{}) || provider.secret != "" {
		t.Fatal("invalid environment credentials reached the connection provider")
	}
}

func TestAuthStatusReportsEnvironmentSourceWithoutSecret(t *testing.T) {
	t.Parallel()

	source := &staticCredentialSource{credentials: environmentPasswordCredentials(), present: true}
	client := &cliConnectionClient{response: "PONG"}
	provider := &cliConnectionProvider{method: profile.AuthMethodPassword, client: client}
	command := New(
		buildinfo.Info{},
		WithConnectionService(connection.NewService(nil, nil, provider)),
		WithEnvironmentCredentialSource(source),
	)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"auth", "status"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Authenticated",
		"Source: environment",
		"User: environment-user",
		"ferrics://environment.example.com:6388",
		"Method: password",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("status output = %q, want %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), source.credentials.Secret) {
		t.Fatalf("status exposed environment secret: %q", output.String())
	}
	if !client.closed {
		t.Fatal("status did not close the environment connection")
	}
}

func TestAuthLogoutDoesNotModifyEnvironmentOrSavedCredentials(t *testing.T) {
	t.Parallel()

	profiles := &cliProfileStore{values: map[string]profile.Profile{
		"default": {
			Name: "default",
			Authentication: profile.Authentication{
				Method: profile.AuthMethodPassword,
			},
		},
	}}
	credentials := &cliCredentialStore{values: map[string]string{"default": "saved-secret"}}
	source := &staticCredentialSource{credentials: environmentPasswordCredentials(), present: true}
	command := New(
		buildinfo.Info{},
		WithLoginService(auth.NewService(profiles, credentials)),
		WithCredentialStore(credentials),
		WithEnvironmentCredentialSource(source),
	)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"auth", "logout"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Environment credentials cannot be logged out") {
		t.Fatalf("logout output = %q", output.String())
	}
	if credentials.values["default"] != "saved-secret" {
		t.Fatal("environment logout modified a saved credential")
	}
	if source.calls != 0 {
		t.Fatal("environment logout read the environment secret")
	}
}

func TestExplicitProfileLogoutOverridesEnvironmentCredentials(t *testing.T) {
	t.Parallel()

	profiles := &cliProfileStore{values: map[string]profile.Profile{
		"default": {
			Name: "default",
			Authentication: profile.Authentication{
				Method: profile.AuthMethodPassword,
			},
		},
	}}
	credentials := &cliCredentialStore{values: map[string]string{"default": "saved-secret"}}
	source := &staticCredentialSource{credentials: environmentPasswordCredentials(), present: true}
	command := New(
		buildinfo.Info{},
		WithLoginService(auth.NewService(profiles, credentials)),
		WithCredentialStore(credentials),
		WithEnvironmentCredentialSource(source),
	)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"--profile", "default", "auth", "logout"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if output.String() != "Logged out.\n" {
		t.Fatalf("logout output = %q", output.String())
	}
	if _, ok := credentials.values["default"]; ok {
		t.Fatal("explicit profile logout retained the saved credential")
	}
	if source.calls != 0 {
		t.Fatal("explicit profile logout resolved environment credentials")
	}
}
