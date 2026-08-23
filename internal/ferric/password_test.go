package ferric

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ferricstore/command-line/internal/profile"
)

const testHTTPEncoding = "ferricstore-json-v1"

func TestPasswordClientFactoryUsesHTTPSBasicAuthAndBasePath(t *testing.T) {
	t.Parallel()

	type receivedRequest struct {
		path    string
		auth    string
		headers http.Header
		proto   int
	}
	received := make(chan receivedRequest, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		received <- receivedRequest{
			path:    request.URL.Path,
			auth:    request.Header.Get("Authorization"),
			headers: request.Header.Clone(),
			proto:   request.ProtoMajor,
		}
		writeHTTPResult(t, writer, http.StatusOK, "PONG")
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	caFile := writeTestCA(t, server.Certificate())
	client, err := (PasswordClientFactory{}).NewPasswordClient(context.Background(), profile.Profile{
		URL:        server.URL + "/lambda/prod",
		CACertFile: caFile,
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}, "super-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	if pong, err := client.Ping(context.Background()); err != nil || pong != "PONG" {
		t.Fatalf("Ping() = %q, %v", pong, err)
	}
	request := <-received
	if request.path != "/lambda/prod/v1/commands" {
		t.Fatalf("request path = %q", request.path)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("operator:super-secret"))
	if request.auth != wantAuth {
		t.Fatalf("Authorization = %q", request.auth)
	}
	if request.proto != 2 {
		t.Fatalf("HTTP protocol major = %d, want 2", request.proto)
	}
	for name := range request.headers {
		if strings.Contains(strings.ToLower(name), "tenant") {
			t.Fatalf("HTTP request included tenant header %q", name)
		}
	}
}

func TestPasswordClientFactoryRejectsPlainHTTPBeforeNetwork(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	client, err := (PasswordClientFactory{}).NewPasswordClient(context.Background(), profile.Profile{
		URL: server.URL,
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}, "super-secret")
	if client != nil {
		_ = client.Close()
		t.Fatal("NewPasswordClient() returned a client for plaintext credentials")
	}
	if err == nil || !strings.Contains(err.Error(), "https://") {
		t.Fatalf("NewPasswordClient() error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("plaintext endpoint received %d request(s)", requests.Load())
	}
}

func TestPasswordClientFactoryRejectsInvalidCustomCA(t *testing.T) {
	t.Parallel()

	caFile := t.TempDir() + "/invalid-ca.pem"
	if err := os.WriteFile(caFile, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := (PasswordClientFactory{}).NewPasswordClient(context.Background(), profile.Profile{
		URL:        "https://store.example.com",
		CACertFile: caFile,
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}, "super-secret")
	if client != nil {
		_ = client.Close()
		t.Fatal("NewPasswordClient() returned a client for an invalid CA")
	}
	if err == nil || !strings.Contains(err.Error(), "CA certificate") {
		t.Fatalf("NewPasswordClient() error = %v", err)
	}
}

func TestPasswordClientFactoryRejectsRelativeCustomCAPath(t *testing.T) {
	t.Parallel()

	client, err := (PasswordClientFactory{}).NewPasswordClient(context.Background(), profile.Profile{
		URL:        "https://store.example.com",
		CACertFile: "certificates/private-ca.pem",
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}, "super-secret")
	if client != nil {
		_ = client.Close()
		t.Fatal("NewPasswordClient() returned a client for a relative CA path")
	}
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("NewPasswordClient() error = %v, want absolute-path guidance", err)
	}
}

func TestPasswordClientFactoryBoundsAndValidatesCAFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path func(*testing.T) string
		want string
	}{
		{
			name: "too large",
			path: func(t *testing.T) string {
				path := t.TempDir() + "/large-ca.pem"
				if err := os.WriteFile(path, bytes.Repeat([]byte("x"), maxCACertificateBytes+1), 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			},
			want: "exceeds",
		},
		{
			name: "not regular",
			path: func(t *testing.T) string { return t.TempDir() },
			want: "regular file",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client, err := (PasswordClientFactory{}).NewPasswordClient(context.Background(), profile.Profile{
				URL:        "https://store.example.com",
				CACertFile: test.path(t),
				Authentication: profile.Authentication{
					Method:   profile.AuthMethodPassword,
					Username: "operator",
				},
			}, "super-secret")
			if client != nil {
				_ = client.Close()
				t.Fatal("NewPasswordClient() returned a client for an unsafe CA path")
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewPasswordClient() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPasswordClientFactoryDoesNotForwardCredentialsAcrossRedirects(t *testing.T) {
	t.Parallel()

	var targetRequests atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		targetRequests.Add(1)
		writeHTTPResult(t, writer, http.StatusOK, "PONG")
	}))
	defer target.Close()

	redirect := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", target.URL+"/v1/commands")
		writer.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	caFile := writeTestCA(t, redirect.Certificate(), target.Certificate())
	client, err := (PasswordClientFactory{}).NewPasswordClient(context.Background(), profile.Profile{
		URL:        redirect.URL,
		CACertFile: caFile,
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}, "super-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Ping(context.Background()); err == nil {
		t.Fatal("Ping() followed a redirect")
	}
	if targetRequests.Load() != 0 {
		t.Fatalf("redirect target received %d request(s)", targetRequests.Load())
	}
}

func TestPasswordClientFactoryUsesCallerDeadlineAsHTTPTimeout(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{}, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requestStarted <- struct{}{}
		time.Sleep(500 * time.Millisecond)
		writeHTTPResult(t, writer, http.StatusOK, "PONG")
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	client, err := (PasswordClientFactory{}).NewPasswordClient(ctx, profile.Profile{
		URL:        server.URL,
		CACertFile: writeTestCA(t, server.Certificate()),
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}, "super-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	started := time.Now()
	_, pingErr := client.Ping(context.Background())
	if pingErr == nil {
		t.Fatal("Ping() ignored the connection context deadline")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Ping() exceeded caller timeout: %s", elapsed)
	}
	select {
	case <-requestStarted:
	default:
		t.Fatal("timeout test did not reach the HTTP endpoint")
	}
}

func writeTestCA(t *testing.T, certificates ...*x509.Certificate) string {
	t.Helper()
	path := t.TempDir() + "/ca.pem"
	contents := make([]byte, 0)
	for _, certificate := range certificates {
		contents = append(contents, pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certificate.Raw,
		})...)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeHTTPResult(t *testing.T, writer http.ResponseWriter, status int, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(map[string]any{
		"encoding": testHTTPEncoding,
		"results": []any{map[string]any{
			"status": "ok",
			"value":  value,
		}},
	}); err != nil {
		t.Errorf("encode HTTP response: %v", err)
	}
}
