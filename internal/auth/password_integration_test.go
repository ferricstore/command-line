//go:build integration

package auth

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/ferric"
	"github.com/ferricstore/command-line/internal/profile"
	ferricstore "github.com/ferricstore/ferricstore-go"
)

func TestIntegrationOSSBootstrap(t *testing.T) {
	if os.Getenv("FERRICSTORE_OSS_BOOTSTRAP") != "1" {
		t.Skip("OSS login bootstrap is run by scripts/integration-login-oss.sh")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	address := requiredIntegrationEnv(t, "FERRICSTORE_OSS_ADDR")
	username := requiredIntegrationEnv(t, "FERRICSTORE_OSS_USERNAME")
	password := requiredIntegrationEnv(t, "FERRICSTORE_OSS_PASSWORD")

	bootstrap := ferricstore.NewClient(address)
	setUserErr := bootstrap.ACLSetUser(ctx, username, "on", ">"+password, "+@all", "~*")
	_ = bootstrap.Close()

	client := ferricstore.NewClient(
		address,
		ferricstore.WithNativeOptions(
			ferricstore.WithNativeCredentials(username, password),
		),
	)
	defer client.Close()
	if pong, err := client.Ping(ctx); err != nil || pong != "PONG" {
		t.Fatalf("authenticated bootstrap PING = %q, %v (SETUSER error %v)", pong, err, setUserErr)
	}
	if err := client.ACLSave(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationOSSLogin(t *testing.T) {
	if os.Getenv("FERRICSTORE_OSS_LOGIN_TEST") != "1" {
		t.Skip("OSS login integration is run by scripts/integration-login-oss.sh")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rawURL := "ferric://" + requiredIntegrationEnv(t, "FERRICSTORE_OSS_ADDR")
	username := requiredIntegrationEnv(t, "FERRICSTORE_OSS_USERNAME")
	password := requiredIntegrationEnv(t, "FERRICSTORE_OSS_PASSWORD")
	profiles := newMemoryProfileStore()
	credentials := newMemoryCredentialStore()
	loginService := NewService(
		profiles,
		credentials,
		NewPasswordProvider(ferric.PasswordValidator{}),
	)

	result, err := loginService.Login(ctx, LoginRequest{
		ProfileName: "oss-integration",
		Method:      profile.AuthMethodPassword,
		URL:         rawURL,
		Username:    username,
		Secret:      password,
		Store:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Stored || result.Principal != username {
		t.Fatalf("Login() = %#v", result)
	}
	if credentials.values["oss-integration"] != password {
		t.Fatal("validated OSS credential was not persisted")
	}

	connections := connection.NewService(
		profiles,
		credentials,
		connection.NewPasswordProvider(ferric.PasswordClientFactory{}),
	)
	client, connectedProfile, err := connections.Open(ctx, "oss-integration")
	if err != nil {
		t.Fatal(err)
	}
	pong, pingErr := client.Ping(ctx)
	closeErr := client.Close()
	if err := errors.Join(pingErr, closeErr); err != nil || pong != "PONG" {
		t.Fatalf("saved-profile PING = %q, %v", pong, err)
	}
	if connectedProfile.Name != "oss-integration" {
		t.Fatalf("connected profile = %#v", connectedProfile)
	}

	_, err = loginService.Login(ctx, LoginRequest{
		ProfileName: "wrong-password",
		Method:      profile.AuthMethodPassword,
		URL:         rawURL,
		Username:    username,
		Secret:      "definitely-wrong",
		Store:       true,
	})
	if err == nil {
		t.Fatal("login with an invalid OSS password succeeded")
	}
	if _, ok := credentials.values["wrong-password"]; ok {
		t.Fatal("invalid OSS credential was persisted")
	}
}

func requiredIntegrationEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}
