package connection

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ferricstore/command-line/internal/profile"
)

const maxEnvironmentSecretBytes = 64 * 1024

const (
	envURL          = "FERRIC_URL"
	envUsername     = "FERRIC_USERNAME"
	envPassword     = "FERRIC_PASSWORD"
	envPasswordFile = "FERRIC_PASSWORD_FILE"
	envCACertFile   = "FERRIC_CA_CERT_FILE"

	envControlURL   = "FERRIC_CONTROL_URL"
	envOrganization = "FERRIC_ORGANIZATION"
	envCluster      = "FERRIC_CLUSTER"
	envAPIToken     = "FERRIC_API_TOKEN"
	envAPITokenFile = "FERRIC_API_TOKEN_FILE"
)

// EphemeralCredentials contains connection metadata and a secret that must not
// be persisted by the CLI.
type EphemeralCredentials struct {
	Profile profile.Profile
	Secret  string
}

// CredentialSource resolves optional non-persistent connection credentials.
type CredentialSource interface {
	Configured() bool
	Resolve(context.Context) (EphemeralCredentials, bool, error)
}

type environmentLookup func(string) (string, bool)
type secretFileReader func(context.Context, string) ([]byte, error)

// EnvironmentCredentialSource resolves OSS passwords and Enterprise machine
// tokens from process environment variables or mounted secret files.
type EnvironmentCredentialSource struct {
	lookup   environmentLookup
	readFile secretFileReader
}

// NewEnvironmentCredentialSource constructs a source backed by the process
// environment and local filesystem.
func NewEnvironmentCredentialSource() *EnvironmentCredentialSource {
	return newEnvironmentCredentialSource(os.LookupEnv, readBoundedSecretFile)
}

func newEnvironmentCredentialSource(
	lookup environmentLookup,
	readFile secretFileReader,
) *EnvironmentCredentialSource {
	return &EnvironmentCredentialSource{lookup: lookup, readFile: readFile}
}

// Configured reports whether any direct credential variable is present.
func (s *EnvironmentCredentialSource) Configured() bool {
	_, present := s.values()
	return present
}

// Resolve returns one complete environment credential set when configured.
func (s *EnvironmentCredentialSource) Resolve(ctx context.Context) (EphemeralCredentials, bool, error) {
	values, present := s.values()
	if !present {
		return EphemeralCredentials{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return EphemeralCredentials{}, true, err
	}

	ossPresent := anySet(values, envURL, envUsername, envPassword, envPasswordFile, envCACertFile)
	enterprisePresent := anySet(
		values,
		envControlURL,
		envOrganization,
		envCluster,
		envAPIToken,
		envAPITokenFile,
	)
	if ossPresent && enterprisePresent {
		return EphemeralCredentials{}, true, errors.New(
			"OSS password and Enterprise API-token environment variables cannot be combined",
		)
	}
	if ossPresent {
		credentials, err := s.resolvePassword(ctx, values)
		return credentials, true, err
	}
	credentials, err := s.resolveAPIToken(ctx, values)
	return credentials, true, err
}

func (s *EnvironmentCredentialSource) values() (map[string]string, bool) {
	keys := environmentCredentialKeys()
	values := make(map[string]string, len(keys))
	present := false
	if s == nil || s.lookup == nil {
		return values, false
	}
	for _, key := range keys {
		if value, ok := s.lookup(key); ok {
			values[key] = value
			present = true
		}
	}
	return values, present
}

func environmentCredentialKeys() []string {
	return []string{
		envURL,
		envUsername,
		envPassword,
		envPasswordFile,
		envCACertFile,
		envControlURL,
		envOrganization,
		envCluster,
		envAPIToken,
		envAPITokenFile,
	}
}

func (s *EnvironmentCredentialSource) resolvePassword(
	ctx context.Context,
	values map[string]string,
) (EphemeralCredentials, error) {
	rawURL, err := requiredEnvironmentValue(values, envURL)
	if err != nil {
		return EphemeralCredentials{}, err
	}
	username, err := requiredEnvironmentValue(values, envUsername)
	if err != nil {
		return EphemeralCredentials{}, err
	}
	secret, err := s.resolveSecret(ctx, values, envPassword, envPasswordFile)
	if err != nil {
		return EphemeralCredentials{}, err
	}
	caCertFile, err := optionalEnvironmentValue(values, envCACertFile)
	if err != nil {
		return EphemeralCredentials{}, err
	}
	if caCertFile != "" && !filepath.IsAbs(caCertFile) {
		return EphemeralCredentials{}, fmt.Errorf("%s must be an absolute path", envCACertFile)
	}
	return EphemeralCredentials{
		Profile: profile.Profile{
			URL:        rawURL,
			CACertFile: caCertFile,
			Authentication: profile.Authentication{
				Method:   profile.AuthMethodPassword,
				Username: username,
			},
		},
		Secret: secret,
	}, nil
}

func (s *EnvironmentCredentialSource) resolveAPIToken(
	ctx context.Context,
	values map[string]string,
) (EphemeralCredentials, error) {
	controlURL, err := requiredEnvironmentValue(values, envControlURL)
	if err != nil {
		return EphemeralCredentials{}, err
	}
	organization, err := requiredEnvironmentValue(values, envOrganization)
	if err != nil {
		return EphemeralCredentials{}, err
	}
	cluster, err := requiredEnvironmentValue(values, envCluster)
	if err != nil {
		return EphemeralCredentials{}, err
	}
	secret, err := s.resolveSecret(ctx, values, envAPIToken, envAPITokenFile)
	if err != nil {
		return EphemeralCredentials{}, err
	}
	return EphemeralCredentials{
		Profile: profile.Profile{
			ControlURL:   controlURL,
			Organization: organization,
			Cluster:      cluster,
			Authentication: profile.Authentication{
				Method: profile.AuthMethodEnterpriseAPIToken,
			},
		},
		Secret: secret,
	}, nil
}

func (s *EnvironmentCredentialSource) resolveSecret(
	ctx context.Context,
	values map[string]string,
	inlineName string,
	fileName string,
) (string, error) {
	inline, inlineSet := values[inlineName]
	path, fileSet := values[fileName]
	if inlineSet && fileSet {
		return "", fmt.Errorf("%s and %s cannot both be set", inlineName, fileName)
	}
	if !inlineSet && !fileSet {
		return "", fmt.Errorf("%s or %s is required", inlineName, fileName)
	}
	if inlineSet {
		return validateEnvironmentSecret(inline, inlineName)
	}

	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%s must not be empty", fileName)
	}
	if s.readFile == nil {
		return "", fmt.Errorf("read %s: secret-file access is not configured", fileName)
	}
	contents, err := s.readFile(ctx, path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", fileName, err)
	}
	if len(contents) > maxEnvironmentSecretBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", fileName, maxEnvironmentSecretBytes)
	}
	secret := string(contents)
	secret = strings.TrimSuffix(secret, "\n")
	secret = strings.TrimSuffix(secret, "\r")
	return validateEnvironmentSecret(secret, fileName)
}

func requiredEnvironmentValue(values map[string]string, name string) (string, error) {
	value, ok := values[name]
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func optionalEnvironmentValue(values map[string]string, name string) (string, error) {
	value, ok := values[name]
	if !ok {
		return "", nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s must not be empty", name)
	}
	return value, nil
}

func validateEnvironmentSecret(secret, name string) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("%s must not be empty", name)
	}
	if len(secret) > maxEnvironmentSecretBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", name, maxEnvironmentSecretBytes)
	}
	return secret, nil
}

func anySet(values map[string]string, names ...string) bool {
	for _, name := range names {
		if _, ok := values[name]; ok {
			return true
		}
	}
	return false
}

func readBoundedSecretFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	contents, readErr := io.ReadAll(io.LimitReader(file, maxEnvironmentSecretBytes+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return contents, nil
}
