package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/auth"
	"github.com/ferricstore/command-line/internal/buildinfo"
	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/credential"
	"github.com/ferricstore/command-line/internal/profile"
)

type cliProfileStore struct {
	values map[string]profile.Profile
}

func (s *cliProfileStore) Put(_ context.Context, value profile.Profile) error {
	s.values[value.Name] = value
	return nil
}

func (s *cliProfileStore) Get(_ context.Context, name string) (profile.Profile, error) {
	value, ok := s.values[name]
	if !ok {
		return profile.Profile{}, profile.ErrNotFound
	}
	return value, nil
}

type cliCredentialStore struct {
	values map[string]string
}

func (s *cliCredentialStore) Put(_ context.Context, name, secret string) error {
	s.values[name] = secret
	return nil
}

func (s *cliCredentialStore) Get(_ context.Context, name string) (string, error) {
	value, ok := s.values[name]
	if !ok {
		return "", credential.ErrNotFound
	}
	return value, nil
}

func (s *cliCredentialStore) Delete(_ context.Context, name string) error {
	if _, ok := s.values[name]; !ok {
		return credential.ErrNotFound
	}
	delete(s.values, name)
	return nil
}

type cliValidator struct {
	err      error
	username string
	password string
}

func (v *cliValidator) ValidatePassword(_ context.Context, _, username, password string) error {
	v.username = username
	v.password = password
	return v.err
}

type staticPasswordReader struct {
	password string
}

type cliEnterpriseValidator struct {
	profile profile.Profile
	token   string
}

func (v *cliEnterpriseValidator) ValidateEnterpriseToken(
	_ context.Context,
	storedProfile profile.Profile,
	token string,
) (string, error) {
	v.profile = storedProfile
	v.token = token
	return "deploy-bot", nil
}

func TestLoginCommandStoresPlatformTokenForExchangeProvider(t *testing.T) {
	t.Parallel()

	profiles := &cliProfileStore{values: make(map[string]profile.Profile)}
	credentials := &cliCredentialStore{values: make(map[string]string)}
	validator := &cliEnterpriseValidator{}
	service := auth.NewService(
		profiles,
		credentials,
		auth.NewEnterpriseTokenProvider(profile.AuthMethodEnterpriseAPIToken, validator),
	)
	command := New(buildinfo.Info{}, WithLoginService(service))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetIn(strings.NewReader("fsp_sa_control-secret\n"))
	command.SetArgs([]string{
		"auth", "login",
		"--profile", "production",
		"--method", "enterprise-api-token",
		"--control-url", "https://platform.example.com",
		"--organization", "acme",
		"--cluster", "cluster-id",
		"--token-stdin",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	storedProfile := profiles.values["production"]
	if storedProfile.ControlURL != "https://platform.example.com" || storedProfile.Cluster != "cluster-id" {
		t.Fatalf("stored profile = %#v", storedProfile)
	}
	if credentials.values[storedProfile.CredentialReference()] != "fsp_sa_control-secret" {
		t.Fatal("Platform token was not stored in the credential store")
	}
	if validator.profile.URL != "" || validator.token != "fsp_sa_control-secret" {
		t.Fatalf("validator input = %#v/%q", validator.profile, validator.token)
	}
	if strings.Contains(output.String(), validator.token) {
		t.Fatal("command output exposed the Platform token")
	}
}

func (r staticPasswordReader) ReadPassword(_ io.Reader, _ io.Writer) (string, error) {
	return r.password, nil
}

func TestLoginCommandReadsPasswordFromStdinAndStores(t *testing.T) {
	t.Parallel()

	profiles := &cliProfileStore{values: make(map[string]profile.Profile)}
	credentials := &cliCredentialStore{values: make(map[string]string)}
	validator := &cliValidator{}
	service := auth.NewService(profiles, credentials, auth.NewPasswordProvider(validator))
	command := New(buildinfo.Info{}, WithLoginService(service))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetIn(strings.NewReader("super-secret\n"))
	command.SetArgs([]string{
		"auth", "login",
		"--profile", "production",
		"--url", "ferric://store.example.com:6388",
		"--username", "operator",
		"--password-stdin",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if validator.password != "super-secret" || validator.username != "operator" {
		t.Fatalf("validator received %q/%q", validator.username, validator.password)
	}
	storedProfile := profiles.values["production"]
	if credentials.values[storedProfile.CredentialReference()] != "super-secret" {
		t.Fatal("credential was not stored")
	}
	if strings.Contains(output.String(), "super-secret") {
		t.Fatal("command output exposed the password")
	}
	if !strings.Contains(output.String(), "saved profile \"production\"") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestLoginCommandNoStore(t *testing.T) {
	t.Parallel()

	profiles := &cliProfileStore{values: make(map[string]profile.Profile)}
	credentials := &cliCredentialStore{values: make(map[string]string)}
	service := auth.NewService(profiles, credentials, auth.NewPasswordProvider(&cliValidator{}))
	command := New(buildinfo.Info{}, WithLoginService(service))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetIn(strings.NewReader("secret\n"))
	command.SetArgs([]string{"auth", "login", "--password-stdin", "--no-store"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(profiles.values) != 0 || len(credentials.values) != 0 {
		t.Fatal("--no-store persisted login state")
	}
	if !strings.Contains(output.String(), "credentials were not stored") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestLoginCommandAuthenticationFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("invalid username-password pair")
	profiles := &cliProfileStore{values: make(map[string]profile.Profile)}
	credentials := &cliCredentialStore{values: make(map[string]string)}
	service := auth.NewService(profiles, credentials, auth.NewPasswordProvider(&cliValidator{err: want}))
	command := New(buildinfo.Info{}, WithLoginService(service))
	command.SetIn(strings.NewReader("wrong\n"))
	command.SetArgs([]string{"auth", "login", "--password-stdin"})

	err := command.Execute()
	if !errors.Is(err, want) {
		t.Fatalf("Execute() error = %v, want authentication failure", err)
	}
	if len(profiles.values) != 0 || len(credentials.values) != 0 {
		t.Fatal("failed login persisted state")
	}
}

func TestLoginCommandUsesInteractiveReader(t *testing.T) {
	t.Parallel()

	validator := &cliValidator{}
	service := auth.NewService(
		&cliProfileStore{values: make(map[string]profile.Profile)},
		&cliCredentialStore{values: make(map[string]string)},
		auth.NewPasswordProvider(validator),
	)
	command := New(
		buildinfo.Info{},
		WithLoginService(service),
		WithPasswordReader(staticPasswordReader{password: "interactive-secret"}),
	)
	command.SetArgs([]string{"auth", "login", "--no-store"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if validator.password != "interactive-secret" {
		t.Fatalf("password = %q", validator.password)
	}
}

func TestAuthStatusVerifiesConnectionWithoutExposingCredential(t *testing.T) {
	t.Setenv("FERRIC_PROFILE", "")

	manager := profile.NewFileStore(filepath.Join(t.TempDir(), "config.json"))
	storedProfile := profile.Profile{
		Name: "default",
		URL:  "ferric://store.example.com:6388",
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}
	if err := manager.Put(context.Background(), storedProfile); err != nil {
		t.Fatal(err)
	}
	credentials := &cliCredentialStore{values: map[string]string{"default": "super-secret"}}
	client := &cliConnectionClient{response: "PONG"}
	provider := &cliConnectionProvider{method: profile.AuthMethodPassword, client: client}
	connections := connection.NewService(manager, credentials, provider)
	command := New(
		buildinfo.Info{},
		WithProfileManager(manager),
		WithCredentialStore(credentials),
		WithConnectionService(connections),
	)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"auth", "status"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Authenticated", "Source: profile", "User: operator", storedProfile.URL, "Method: password"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("status output = %q, want %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "super-secret") {
		t.Fatalf("status exposed credential: %q", output.String())
	}
	if !client.closed {
		t.Fatal("status did not close the connection")
	}

	want := errors.New("authentication rejected")
	failedClient := &cliConnectionClient{pingErr: want}
	failedProvider := &cliConnectionProvider{method: profile.AuthMethodPassword, client: failedClient}
	failed := New(
		buildinfo.Info{},
		WithProfileManager(manager),
		WithCredentialStore(credentials),
		WithConnectionService(connection.NewService(manager, credentials, failedProvider)),
	)
	failed.SetArgs([]string{"auth", "status"})
	if err := failed.Execute(); !errors.Is(err, want) {
		t.Fatalf("failed status error = %v", err)
	}
	if !failedClient.closed {
		t.Fatal("failed status did not close the connection")
	}
}

func TestAuthLogoutKeepsProfileAndIsIdempotent(t *testing.T) {
	t.Setenv("FERRIC_PROFILE", "")

	manager := profile.NewFileStore(filepath.Join(t.TempDir(), "config.json"))
	storedProfile := profile.Profile{
		Name: "default",
		URL:  "ferric://store.example.com:6388",
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}
	if err := manager.Put(context.Background(), storedProfile); err != nil {
		t.Fatal(err)
	}
	credentials := &cliCredentialStore{values: map[string]string{"default": "super-secret"}}
	service := auth.NewService(manager, credentials)

	first := New(
		buildinfo.Info{},
		WithLoginService(service),
		WithProfileManager(manager),
		WithCredentialStore(credentials),
	)
	var firstOutput bytes.Buffer
	first.SetOut(&firstOutput)
	first.SetErr(&firstOutput)
	first.SetArgs([]string{"auth", "logout"})
	if err := first.Execute(); err != nil {
		t.Fatal(err)
	}
	if firstOutput.String() != "Logged out.\n" {
		t.Fatalf("first logout output = %q", firstOutput.String())
	}
	if _, ok := credentials.values["default"]; ok {
		t.Fatal("logout retained the credential")
	}
	if got, err := manager.Get(context.Background(), "default"); err != nil || got != storedProfile {
		t.Fatalf("profile after logout = %#v, %v", got, err)
	}

	second := New(
		buildinfo.Info{},
		WithLoginService(service),
		WithProfileManager(manager),
		WithCredentialStore(credentials),
	)
	var secondOutput bytes.Buffer
	second.SetOut(&secondOutput)
	second.SetErr(&secondOutput)
	second.SetArgs([]string{"auth", "logout"})
	if err := second.Execute(); err != nil {
		t.Fatal(err)
	}
	if secondOutput.String() != "Already logged out.\n" {
		t.Fatalf("second logout output = %q", secondOutput.String())
	}
}

func TestDependencyOptionsComposeDefaultAuthService(t *testing.T) {
	t.Setenv("FERRIC_PROFILE", "")

	manager := profile.NewFileStore(filepath.Join(t.TempDir(), "config.json"))
	if err := manager.Put(context.Background(), profile.Profile{
		Name: "production",
		URL:  "ferric://store.example.com:6388",
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Use(context.Background(), "production"); err != nil {
		t.Fatal(err)
	}
	credentials := &cliCredentialStore{values: map[string]string{"production": "super-secret"}}
	command := New(
		buildinfo.Info{},
		WithProfileManager(manager),
		WithCredentialStore(credentials),
	)
	command.SetArgs([]string{"auth", "logout"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, ok := credentials.values["production"]; ok {
		t.Fatal("default auth service did not use the configured credential store")
	}
}
