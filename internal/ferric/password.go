package ferric

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	ferricstore "github.com/ferricstore/ferricstore-go"
)

const (
	defaultHTTPPasswordTimeout = 30 * time.Second
	maxCACertificateBytes      = 1024 * 1024
	maxHTTPConnections         = 100
)

// PasswordClientFactory constructs SDK clients that authenticate with OSS ACL credentials.
type PasswordClientFactory struct{}

// NewPasswordClient returns a lazily connected SDK client. Native credentials
// are applied to every TCP connection; HTTP credentials are applied to every
// HTTPS request.
func (PasswordClientFactory) NewPasswordClient(
	ctx context.Context,
	storedProfile profile.Profile,
	password string,
) (connection.Client, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	parsed, err := url.Parse(storedProfile.URL)
	if err != nil {
		return nil, errors.New("FerricStore URL is invalid")
	}
	if parsed.User != nil {
		return nil, errors.New("FerricStore URL must not contain credentials")
	}
	if storedProfile.CACertFile != "" && !filepath.IsAbs(storedProfile.CACertFile) {
		return nil, errors.New("CA certificate path must be absolute")
	}

	username := storedProfile.Authentication.Username
	switch strings.ToLower(parsed.Scheme) {
	case "ferric":
		if storedProfile.CACertFile != "" {
			return nil, errors.New("a CA certificate requires a TLS endpoint")
		}
		return ferricstore.NewClientFromURL(
			storedProfile.URL,
			ferricstore.WithNativeOptions(
				ferricstore.WithNativeCredentials(username, password),
			),
		)
	case "ferrics":
		tlsConfig, err := passwordTLSConfig(storedProfile.CACertFile)
		if err != nil {
			return nil, err
		}
		return ferricstore.NewClientFromURL(
			storedProfile.URL,
			ferricstore.WithNativeOptions(
				ferricstore.WithNativeCredentials(username, password),
				ferricstore.WithNativeTLS(tlsConfig),
			),
		)
	case "https":
		return newHTTPPasswordClient(ctx, storedProfile, password)
	case "http":
		return nil, errors.New("FerricStore username/password authentication requires an https:// URL")
	default:
		return nil, errors.New("FerricStore URL must use ferric://, ferrics://, or https://")
	}
}

func newHTTPPasswordClient(
	ctx context.Context,
	storedProfile profile.Profile,
	password string,
) (connection.Client, error) {
	tlsConfig, err := passwordTLSConfig(storedProfile.CACertFile)
	if err != nil {
		return nil, err
	}
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("default HTTP transport is not configurable")
	}
	transport := defaultTransport.Clone()
	transport.TLSClientConfig = tlsConfig
	transport.ForceAttemptHTTP2 = true
	transport.MaxIdleConns = maxHTTPConnections
	transport.MaxIdleConnsPerHost = maxHTTPConnections
	transport.MaxConnsPerHost = maxHTTPConnections
	httpClient := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	client, err := ferricstore.NewClientFromURL(
		storedProfile.URL,
		ferricstore.WithHTTPOptions(
			ferricstore.WithHTTPBasicAuth(storedProfile.Authentication.Username, password),
			ferricstore.WithHTTPClient(httpClient),
			ferricstore.WithHTTPTimeout(passwordHTTPTimeout(ctx)),
		),
	)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	return &passwordClient{Client: client, transport: transport}, nil
}

type passwordClient struct {
	*ferricstore.Client
	transport *http.Transport
}

func (c *passwordClient) Close() error {
	if c == nil {
		return nil
	}
	var err error
	if c.Client != nil {
		err = c.Client.Close()
	}
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
	return err
}

func passwordHTTPTimeout(ctx context.Context) time.Duration {
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok {
			if remaining := time.Until(deadline); remaining > 0 {
				return remaining
			}
		}
	}
	return defaultHTTPPasswordTimeout
}

func passwordTLSConfig(caFile string) (*tls.Config, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile == "" {
		return config, nil
	}
	contents, err := readCACertificateFile(caFile)
	if err != nil {
		return nil, err
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("load system CA certificates: %w", err)
	}
	if roots == nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(contents) {
		return nil, errors.New("CA certificate file does not contain a valid PEM certificate")
	}
	config.RootCAs = roots
	return config, nil
}

func readCACertificateFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect CA certificate file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("CA certificate path must identify a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open CA certificate file: %w", err)
	}
	defer func() { _ = file.Close() }()
	contents, err := io.ReadAll(io.LimitReader(file, maxCACertificateBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read CA certificate file: %w", err)
	}
	if len(contents) > maxCACertificateBytes {
		return nil, fmt.Errorf("CA certificate file exceeds %d bytes", maxCACertificateBytes)
	}
	return contents, nil
}

// PasswordValidator validates OSS credentials against a real FerricStore connection.
type PasswordValidator struct{}

// ValidatePassword authenticates and executes a PING.
func (PasswordValidator) ValidatePassword(ctx context.Context, storedProfile profile.Profile, password string) error {
	client, err := (PasswordClientFactory{}).NewPasswordClient(ctx, storedProfile, password)
	if err != nil {
		return err
	}
	_, pingErr := client.Ping(ctx)
	closeErr := client.Close()
	return errors.Join(pingErr, closeErr)
}
