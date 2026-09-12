package connection

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/profile"
)

func TestEnvironmentCredentialSourceIsInactiveWithoutConnectionVariables(t *testing.T) {
	t.Parallel()

	source := newEnvironmentCredentialSource(mapEnvironment(nil), mapSecretFiles(nil, nil))
	if source.Configured() {
		t.Fatal("Configured() = true without environment variables")
	}
	credentials, present, err := source.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if present || credentials != (EphemeralCredentials{}) {
		t.Fatalf("Resolve() = %#v, %t; want inactive", credentials, present)
	}
}

func TestEnvironmentCredentialSourceResolvesOSSPassword(t *testing.T) {
	t.Parallel()
	caCertFile := filepath.Join(t.TempDir(), "ferric-ca.pem")

	source := newEnvironmentCredentialSource(mapEnvironment(map[string]string{
		"FERRIC_URL":          " https://store.example.com/proxy ",
		"FERRIC_USERNAME":     " operator ",
		"FERRIC_PASSWORD":     " secret with spaces ",
		"FERRIC_CA_CERT_FILE": " " + caCertFile + " ",
	}), mapSecretFiles(nil, nil))
	if !source.Configured() {
		t.Fatal("Configured() = false with password environment variables")
	}

	credentials, present, err := source.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Fatal("Resolve() did not report environment credentials")
	}
	wantProfile := profile.Profile{
		URL:        "https://store.example.com/proxy",
		CACertFile: caCertFile,
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}
	if credentials.Profile != wantProfile || credentials.Secret != " secret with spaces " {
		t.Fatalf("Resolve() = %#v", credentials)
	}
}

func TestEnvironmentCredentialSourceRejectsCredentialsInOSSURL(t *testing.T) {
	t.Parallel()

	source := newEnvironmentCredentialSource(mapEnvironment(map[string]string{
		"FERRIC_URL":      "ferric://embedded:secret@store.example.com:6388",
		"FERRIC_USERNAME": "operator",
		"FERRIC_PASSWORD": "environment-secret",
	}), mapSecretFiles(nil, nil))

	credentials, present, err := source.Resolve(context.Background())
	if !present {
		t.Fatal("Resolve() did not report the invalid environment source as present")
	}
	if err == nil || !strings.Contains(err.Error(), "must not contain credentials") {
		t.Fatalf("Resolve() error = %v, want credential-bearing URL rejection", err)
	}
	if credentials != (EphemeralCredentials{}) {
		t.Fatalf("Resolve() returned credentials for an unsafe URL: %#v", credentials)
	}
}

func TestEnvironmentCredentialSourceReadsOSSPasswordFile(t *testing.T) {
	t.Parallel()

	source := newEnvironmentCredentialSource(mapEnvironment(map[string]string{
		"FERRIC_URL":           "ferric://store:6388",
		"FERRIC_USERNAME":      "default",
		"FERRIC_PASSWORD_FILE": "/run/secrets/ferric-password",
	}), mapSecretFiles(map[string][]byte{
		"/run/secrets/ferric-password": []byte("file-secret\r\n"),
	}, nil))

	credentials, present, err := source.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !present || credentials.Secret != "file-secret" {
		t.Fatalf("Resolve() = %#v, %t", credentials, present)
	}
}

func TestEnvironmentCredentialSourceResolvesEnterpriseAPIToken(t *testing.T) {
	t.Parallel()

	source := newEnvironmentCredentialSource(mapEnvironment(map[string]string{
		"FERRIC_CONTROL_URL":  "https://control.example.com",
		"FERRIC_ORGANIZATION": "acme",
		"FERRIC_CLUSTER":      "production",
		"FERRIC_API_TOKEN":    "machine-token",
	}), mapSecretFiles(nil, nil))

	credentials, present, err := source.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantProfile := profile.Profile{
		ControlURL:   "https://control.example.com",
		Organization: "acme",
		Cluster:      "production",
		Authentication: profile.Authentication{
			Method: profile.AuthMethodEnterpriseAPIToken,
		},
	}
	if !present || credentials.Profile != wantProfile || credentials.Secret != "machine-token" {
		t.Fatalf("Resolve() = %#v, %t", credentials, present)
	}
}

func TestEnvironmentCredentialSourceRejectsIncompleteAndConflictingSets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{
			name: "missing OSS URL",
			values: map[string]string{
				"FERRIC_USERNAME": "default",
				"FERRIC_PASSWORD": "secret",
			},
			want: "FERRIC_URL is required",
		},
		{
			name: "missing OSS username",
			values: map[string]string{
				"FERRIC_URL":      "ferric://store:6388",
				"FERRIC_PASSWORD": "secret",
			},
			want: "FERRIC_USERNAME is required",
		},
		{
			name: "missing OSS secret",
			values: map[string]string{
				"FERRIC_URL":      "ferric://store:6388",
				"FERRIC_USERNAME": "default",
			},
			want: "FERRIC_PASSWORD or FERRIC_PASSWORD_FILE is required",
		},
		{
			name: "empty CA certificate path",
			values: map[string]string{
				"FERRIC_URL":          "https://store:8443",
				"FERRIC_USERNAME":     "default",
				"FERRIC_PASSWORD":     "secret",
				"FERRIC_CA_CERT_FILE": " ",
			},
			want: "FERRIC_CA_CERT_FILE must not be empty",
		},
		{
			name: "relative CA certificate path",
			values: map[string]string{
				"FERRIC_URL":          "https://store:8443",
				"FERRIC_USERNAME":     "default",
				"FERRIC_PASSWORD":     "secret",
				"FERRIC_CA_CERT_FILE": "certificates/private-ca.pem",
			},
			want: "FERRIC_CA_CERT_FILE must be an absolute path",
		},
		{
			name: "two OSS secret sources",
			values: map[string]string{
				"FERRIC_URL":           "ferric://store:6388",
				"FERRIC_USERNAME":      "default",
				"FERRIC_PASSWORD":      "secret",
				"FERRIC_PASSWORD_FILE": "/secret",
			},
			want: "FERRIC_PASSWORD and FERRIC_PASSWORD_FILE cannot both be set",
		},
		{
			name: "mixed authentication methods",
			values: map[string]string{
				"FERRIC_URL":          "ferric://store:6388",
				"FERRIC_USERNAME":     "default",
				"FERRIC_PASSWORD":     "secret",
				"FERRIC_CONTROL_URL":  "https://control.example.com",
				"FERRIC_ORGANIZATION": "acme",
				"FERRIC_CLUSTER":      "production",
				"FERRIC_API_TOKEN":    "token",
			},
			want: "OSS password and Enterprise API-token environment variables cannot be combined",
		},
		{
			name: "missing Enterprise metadata",
			values: map[string]string{
				"FERRIC_API_TOKEN": "token",
			},
			want: "FERRIC_CONTROL_URL is required",
		},
		{
			name: "two Enterprise token sources",
			values: map[string]string{
				"FERRIC_CONTROL_URL":    "https://control.example.com",
				"FERRIC_ORGANIZATION":   "acme",
				"FERRIC_CLUSTER":        "production",
				"FERRIC_API_TOKEN":      "token",
				"FERRIC_API_TOKEN_FILE": "/token",
			},
			want: "FERRIC_API_TOKEN and FERRIC_API_TOKEN_FILE cannot both be set",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source := newEnvironmentCredentialSource(mapEnvironment(test.values), mapSecretFiles(nil, nil))
			_, present, err := source.Resolve(context.Background())
			if !present {
				t.Fatal("Resolve() did not report the invalid environment source as present")
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Resolve() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestEnvironmentCredentialSourceRejectsUnsafeSecretFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content []byte
		readErr error
		want    string
	}{
		{name: "empty", content: nil, want: "must not be empty"},
		{name: "too large", content: bytes.Repeat([]byte("x"), 64*1024+1), want: "exceeds 65536 bytes"},
		{name: "read failure", readErr: errors.New("permission denied"), want: "read FERRIC_PASSWORD_FILE"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source := newEnvironmentCredentialSource(mapEnvironment(map[string]string{
				"FERRIC_URL":           "ferric://store:6388",
				"FERRIC_USERNAME":      "default",
				"FERRIC_PASSWORD_FILE": "/run/secrets/password",
			}), mapSecretFiles(map[string][]byte{
				"/run/secrets/password": test.content,
			}, test.readErr))
			_, present, err := source.Resolve(context.Background())
			if !present || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Resolve() present/error = %t/%v, want %q", present, err, test.want)
			}
		})
	}
}

func TestEnvironmentCredentialSourceHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source := newEnvironmentCredentialSource(mapEnvironment(map[string]string{
		"FERRIC_URL":      "ferric://store:6388",
		"FERRIC_USERNAME": "default",
		"FERRIC_PASSWORD": "secret",
	}), mapSecretFiles(nil, nil))
	_, present, err := source.Resolve(ctx)
	if !present || !errors.Is(err, context.Canceled) {
		t.Fatalf("Resolve() present/error = %t/%v", present, err)
	}
}

func mapEnvironment(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func mapSecretFiles(values map[string][]byte, readErr error) func(context.Context, string) ([]byte, error) {
	return func(_ context.Context, path string) ([]byte, error) {
		if readErr != nil {
			return nil, readErr
		}
		return append([]byte(nil), values[path]...), nil
	}
}
