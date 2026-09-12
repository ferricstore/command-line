package platformapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExchangeReturnsTemporaryNativeCredential(t *testing.T) {
	t.Parallel()

	const token = "fsp_user_control-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/credential-exchange/clusters/cluster-id" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatal("missing bearer token")
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"endpoint":       "ferrics://store.example.com:6389",
			"username":       "platform_cli_0123456789abcdef",
			"password":       "native-secret",
			"expires_at":     time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano),
			"namespace":      "tenant:acme:primary",
			"principal":      "person@example.com",
			"principal_type": "user",
		}})
	}))
	defer server.Close()

	credential, err := NewClient(server.Client()).Exchange(
		context.Background(), server.URL, token, "acme", "cluster-id", 5*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Username != "platform_cli_0123456789abcdef" || credential.Password != "native-secret" {
		t.Fatalf("credential = %#v", credential)
	}
}

func TestExchangeRejectsUnsafeControlURLAndRedirects(t *testing.T) {
	t.Parallel()

	client := NewClient(nil)
	if _, err := client.Exchange(context.Background(), "http://platform.example.com", "token", "org", "cluster", 0); err == nil {
		t.Fatal("insecure non-loopback control URL was accepted")
	}

	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalled = true
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	_, err := NewClient(redirect.Client()).Exchange(
		context.Background(), redirect.URL, "fsp_sa_secret", "org", "cluster", 0,
	)
	if err == nil || !strings.Contains(err.Error(), "redirects are not allowed") {
		t.Fatalf("redirect error = %v", err)
	}
	if targetCalled {
		t.Fatal("redirect target was called")
	}
}
