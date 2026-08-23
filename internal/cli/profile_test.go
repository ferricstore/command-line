package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/buildinfo"
	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/credential"
	"github.com/ferricstore/command-line/internal/profile"
)

func TestProfileCommandsManageSavedConnectionsWithoutExposingSecrets(t *testing.T) {
	manager := profile.NewFileStore(filepath.Join(t.TempDir(), "config.json"))
	credentials := &cliCredentialStore{values: map[string]string{
		"default":    "default-secret",
		"production": "production-secret",
	}}
	ctx := context.Background()
	for _, storedProfile := range []profile.Profile{
		{
			Name: "default",
			URL:  "ferric://127.0.0.1:6388",
			Authentication: profile.Authentication{
				Method:   profile.AuthMethodPassword,
				Username: "default",
			},
		},
		{
			Name:       "production",
			URL:        "https://store.example.com/proxy",
			CACertFile: "/etc/ferric/ca.pem",
			Authentication: profile.Authentication{
				Method:   profile.AuthMethodPassword,
				Username: "operator",
			},
		},
	} {
		if err := manager.Put(ctx, storedProfile); err != nil {
			t.Fatal(err)
		}
	}

	listOutput := executeProfileCLI(t, manager, credentials, "profile", "list")
	for _, want := range []string{"CURRENT", "NAME", "ENDPOINT", "default", "production"} {
		if !strings.Contains(listOutput, want) {
			t.Fatalf("profile list output = %q, want %q", listOutput, want)
		}
	}
	if strings.Contains(listOutput, "secret") {
		t.Fatalf("profile list exposed a credential: %q", listOutput)
	}

	showOutput := executeProfileCLI(t, manager, credentials, "profile", "show", "production")
	for _, want := range []string{"Name:", "production", "https://store.example.com/proxy", "/etc/ferric/ca.pem", "operator"} {
		if !strings.Contains(showOutput, want) {
			t.Fatalf("profile show output = %q, want %q", showOutput, want)
		}
	}
	if strings.Contains(showOutput, "production-secret") {
		t.Fatalf("profile show exposed a credential: %q", showOutput)
	}

	useOutput := executeProfileCLI(t, manager, credentials, "profile", "use", "production")
	if !strings.Contains(useOutput, `Default connection is now "production"`) {
		t.Fatalf("profile use output = %q", useOutput)
	}
	current, err := manager.Current(ctx)
	if err != nil || current != "production" {
		t.Fatalf("Current() = %q, %v", current, err)
	}

	completionOutput := executeProfileCLI(t, manager, credentials, "__complete", "profile", "use", "pro")
	if !strings.Contains(completionOutput, "production") || strings.Contains(completionOutput, "default\n") {
		t.Fatalf("profile completion output = %q", completionOutput)
	}
	overrideCompletion := executeProfileCLI(t, manager, credentials, "__complete", "--profile", "pro")
	if !strings.Contains(overrideCompletion, "production") || strings.Contains(overrideCompletion, "default\n") {
		t.Fatalf("profile override completion output = %q", overrideCompletion)
	}

	deleteOutput := executeProfileCLI(t, manager, credentials, "profile", "delete", "production")
	if !strings.Contains(deleteOutput, `Deleted profile "production"`) {
		t.Fatalf("profile delete output = %q", deleteOutput)
	}
	if _, err := manager.Get(ctx, "production"); !errors.Is(err, profile.ErrNotFound) {
		t.Fatalf("deleted profile error = %v", err)
	}
	if _, ok := credentials.values["production"]; ok {
		t.Fatal("profile delete retained its credential")
	}
	current, err = manager.Current(ctx)
	if err != nil || current != "default" {
		t.Fatalf("Current() after delete = %q, %v", current, err)
	}
}

func TestCurrentProfileIsUsedWithoutFlag(t *testing.T) {
	t.Setenv("FERRIC_PROFILE", "")

	manager := profile.NewFileStore(filepath.Join(t.TempDir(), "config.json"))
	ctx := context.Background()
	for _, name := range []string{"default", "production"} {
		if err := manager.Put(ctx, profile.Profile{
			Name: name,
			URL:  "ferric://" + name + ":6388",
			Authentication: profile.Authentication{
				Method:   profile.AuthMethodPassword,
				Username: "operator",
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.Use(ctx, "production"); err != nil {
		t.Fatal(err)
	}
	credentials := &cliCredentialStore{values: map[string]string{
		"default":    "default-secret",
		"production": "production-secret",
	}}
	client := &cliConnectionClient{response: "PONG"}
	provider := &cliConnectionProvider{method: profile.AuthMethodPassword, client: client}
	connections := connection.NewService(manager, credentials, provider)
	command := New(
		buildinfo.Info{},
		WithProfileManager(manager),
		WithCredentialStore(credentials),
		WithConnectionService(connections),
	)
	command.SetArgs([]string{"server", "ping"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if provider.profile.Name != "production" || provider.secret != "production-secret" {
		t.Fatalf("connection used profile/secret %q/%q", provider.profile.Name, provider.secret)
	}
}

type failingDeleteManager struct {
	profile.Manager
	err error
}

func (m *failingDeleteManager) Delete(context.Context, string) error {
	return m.err
}

func TestProfileDeleteRestoresCredentialWhenMetadataDeletionFails(t *testing.T) {
	manager := profile.NewFileStore(filepath.Join(t.TempDir(), "config.json"))
	ctx := context.Background()
	if err := manager.Put(ctx, profile.Profile{Name: "production"}); err != nil {
		t.Fatal(err)
	}
	want := errors.New("profile write failed")
	failingManager := &failingDeleteManager{Manager: manager, err: want}
	credentials := &cliCredentialStore{values: map[string]string{"production": "secret"}}

	err := deleteProfile(ctx, dependencies{profiles: failingManager, credentials: credentials}, "production")
	if !errors.Is(err, want) {
		t.Fatalf("deleteProfile() error = %v", err)
	}
	if credentials.values["production"] != "secret" {
		t.Fatal("failed profile deletion did not restore the credential")
	}
}

func executeProfileCLI(
	t *testing.T,
	manager profile.Manager,
	credentials credential.Store,
	args ...string,
) string {
	t.Helper()
	command := New(
		buildinfo.Info{},
		WithProfileManager(manager),
		WithCredentialStore(credentials),
	)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		t.Fatalf("ferric %s: %v", strings.Join(args, " "), err)
	}
	return output.String()
}
