//go:build integration

package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/auth"
	"github.com/ferricstore/command-line/internal/buildinfo"
	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/ferric"
	"github.com/ferricstore/command-line/internal/profile"
)

func TestIntegrationHTTPUsernamePasswordTLSAndACL(t *testing.T) {
	if os.Getenv("FERRICSTORE_HTTP_CLI_TEST") != "1" {
		t.Skip("HTTP CLI integration is run by scripts/integration-http-tls.sh")
	}

	rawURL := requiredCLIIntegrationEnv(t, "FERRICSTORE_HTTP_URL")
	caFile := requiredCLIIntegrationEnv(t, "FERRICSTORE_HTTP_CA_FILE")
	username := requiredCLIIntegrationEnv(t, "FERRICSTORE_HTTP_USERNAME")
	password := requiredCLIIntegrationEnv(t, "FERRICSTORE_HTTP_PASSWORD")
	restrictedUsername := requiredCLIIntegrationEnv(t, "FERRICSTORE_HTTP_RESTRICTED_USERNAME")
	restrictedPassword := requiredCLIIntegrationEnv(t, "FERRICSTORE_HTTP_RESTRICTED_PASSWORD")

	profiles := profile.NewFileStore(t.TempDir() + "/profiles.json")
	credentials := newIntegrationCredentialStore()
	login := auth.NewService(
		profiles,
		credentials,
		auth.NewPasswordProvider(ferric.PasswordValidator{}),
	)
	loginOptions := []Option{
		WithLoginService(login),
		WithProfileManager(profiles),
		WithCredentialStore(credentials),
	}

	output, err := executeHTTPIntegrationCLI(
		loginOptions,
		password+"\n",
		"--profile", "http-login", "auth", "login",
		"--url", rawURL, "--username", username, "--ca-cert", caFile,
		"--password-stdin", "--no-store",
	)
	if err != nil {
		t.Fatalf("HTTPS login: %v", err)
	}
	if strings.Contains(output, password) {
		t.Fatal("HTTPS login output exposed the password")
	}
	if !strings.Contains(output, "Authenticated as "+username) {
		t.Fatalf("HTTPS login output = %q", output)
	}
	assertNoHTTPLoginState(t, profiles, credentials)

	if _, err := executeHTTPIntegrationCLI(
		loginOptions,
		"definitely-wrong\n",
		"--profile", "bad-password", "auth", "login",
		"--url", rawURL, "--username", username, "--ca-cert", caFile,
		"--password-stdin",
	); err == nil {
		t.Fatal("HTTPS login accepted an invalid password")
	} else if strings.Contains(err.Error(), "definitely-wrong") {
		t.Fatal("HTTPS login error exposed the rejected password")
	}
	assertNoHTTPLoginState(t, profiles, credentials)

	if _, err := executeHTTPIntegrationCLI(
		loginOptions,
		password+"\n",
		"--profile", "missing-ca", "auth", "login",
		"--url", rawURL, "--username", username, "--password-stdin",
	); err == nil {
		t.Fatal("HTTPS login trusted the private server without its CA")
	} else if strings.Contains(err.Error(), password) {
		t.Fatal("TLS validation error exposed the password")
	}
	assertNoHTTPLoginState(t, profiles, credentials)

	loginDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relativeCAFile, err := filepath.Rel(loginDirectory, filepath.Clean(caFile))
	if err != nil {
		t.Fatal(err)
	}
	output, err = executeHTTPIntegrationCLI(
		loginOptions,
		password+"\n",
		"--profile", "http-saved", "auth", "login",
		"--url", rawURL, "--username", username, "--ca-cert", relativeCAFile,
		"--password-stdin",
	)
	if err != nil {
		t.Fatalf("saved HTTPS login: %v", err)
	}
	if strings.Contains(output, password) {
		t.Fatal("saved HTTPS login output exposed the password")
	}
	if !strings.Contains(output, `saved profile "http-saved"`) {
		t.Fatalf("saved HTTPS login output = %q", output)
	}
	storedProfile, err := profiles.Get(context.Background(), "http-saved")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(storedProfile.CACertFile) {
		t.Fatalf("saved HTTPS profile retained relative CA certificate path %q", storedProfile.CACertFile)
	}

	connections := connection.NewService(
		profiles,
		credentials,
		connection.NewPasswordProvider(ferric.PasswordClientFactory{}),
	)
	t.Chdir(t.TempDir())
	commandOptions := []Option{WithConnectionService(connections)}
	if output, err := executeHTTPIntegrationCLI(commandOptions, "", "--profile", "http-saved", "server", "ping"); err != nil || strings.TrimSpace(output) != "PONG" {
		t.Fatalf("saved-profile PING = %q, %v", output, err)
	}

	t.Setenv("FERRIC_URL", rawURL)
	t.Setenv("FERRIC_USERNAME", username)
	t.Setenv("FERRIC_PASSWORD", password)
	t.Setenv("FERRIC_CA_CERT_FILE", caFile)
	connections = connection.NewService(
		nil,
		nil,
		connection.NewPasswordProvider(ferric.PasswordClientFactory{}),
	)
	commandOptions = []Option{WithConnectionService(connections)}

	if output, err := executeHTTPIntegrationCLI(commandOptions, "", "server", "ping"); err != nil || strings.TrimSpace(output) != "PONG" {
		t.Fatalf("full-user PING = %q, %v", output, err)
	}
	key := fmt.Sprintf("cli:http:%d", os.Getpid())
	if output, err := executeHTTPIntegrationCLI(commandOptions, "", "store", "set", key, "allowed"); err != nil || strings.TrimSpace(output) != "OK" {
		t.Fatalf("full-user SET = %q, %v", output, err)
	}
	if output, err := executeHTTPIntegrationCLI(commandOptions, "", "store", "get", key); err != nil || strings.TrimSpace(output) != "allowed" {
		t.Fatalf("full-user GET = %q, %v", output, err)
	}

	t.Setenv("FERRIC_USERNAME", restrictedUsername)
	t.Setenv("FERRIC_PASSWORD", restrictedPassword)
	if output, err := executeHTTPIntegrationCLI(commandOptions, "", "server", "ping"); err != nil || strings.TrimSpace(output) != "PONG" {
		t.Fatalf("restricted-user PING = %q, %v", output, err)
	}
	_, err = executeHTTPIntegrationCLI(commandOptions, "", "store", "set", key, "blocked")
	if err == nil || (!strings.Contains(strings.ToUpper(err.Error()), "NOPERM") && !strings.Contains(strings.ToLower(err.Error()), "permission")) {
		t.Fatalf("restricted-user SET error = %v", err)
	}
}

func executeHTTPIntegrationCLI(options []Option, input string, args ...string) (string, error) {
	command := New(buildinfo.Info{}, options...)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetIn(strings.NewReader(input))
	command.SetArgs(args)
	err := command.Execute()
	return output.String(), err
}

func assertNoHTTPLoginState(
	t *testing.T,
	profiles profile.Manager,
	credentials *integrationCredentialStore,
) {
	t.Helper()
	stored, err := profiles.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	credentials.mu.Lock()
	credentialCount := len(credentials.values)
	credentials.mu.Unlock()
	if len(stored) != 0 || credentialCount != 0 {
		t.Fatalf("failed or non-persistent login stored profiles=%#v credentials=%d", stored, credentialCount)
	}
}
