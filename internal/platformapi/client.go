// Package platformapi implements the narrow Platform credential-exchange contract.
package platformapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ferricstore/command-line/internal/endpoint"
)

const responseLimit = 1 << 20

// Credential is the temporary data-plane credential returned by Platform.
type Credential struct {
	Endpoint      string
	Username      string
	Password      string
	ExpiresAt     time.Time
	Namespace     string
	Principal     string
	PrincipalType string
}

// Client exchanges control-plane tokens without forwarding them to FerricStore.
type Client struct {
	httpClient *http.Client
}

// NewClient constructs a Platform API client.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{httpClient: httpClient}
}

// Exchange exchanges an fsp_user_ or fsp_sa_ bearer token for a native credential.
func (c *Client) Exchange(
	ctx context.Context,
	controlURL string,
	token string,
	organization string,
	clusterID string,
	ttl time.Duration,
) (Credential, error) {
	if c == nil || c.httpClient == nil {
		return Credential{}, errors.New("platform API client is not configured")
	}
	baseURL, err := validateControlURL(controlURL)
	if err != nil {
		return Credential{}, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return Credential{}, errors.New("platform API token is required")
	}
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" || strings.Contains(clusterID, "/") {
		return Credential{}, errors.New("platform cluster ID is invalid")
	}
	if ttl < 0 || ttl > time.Hour || (ttl > 0 && ttl < time.Minute) {
		return Credential{}, errors.New("platform credential TTL must be between one minute and one hour")
	}

	payload := struct {
		Credential struct {
			Organization string `json:"organization,omitempty"`
			TTLSeconds   int64  `json:"ttl_seconds,omitempty"`
		} `json:"credential"`
	}{}
	payload.Credential.Organization = strings.TrimSpace(organization)
	if ttl > 0 {
		payload.Credential.TTLSeconds = int64(ttl / time.Second)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Credential{}, fmt.Errorf("encode Platform credential exchange: %w", err)
	}

	exchangeURL := *baseURL
	exchangeURL.Path = strings.TrimRight(exchangeURL.Path, "/") +
		"/api/v1/credential-exchange/clusters/" + url.PathEscape(clusterID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, exchangeURL.String(), bytes.NewReader(body))
	if err != nil {
		return Credential{}, fmt.Errorf("create Platform credential exchange: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	httpClient := *c.httpClient
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return errors.New("platform credential exchange redirects are not allowed")
	}
	response, err := httpClient.Do(req)
	if err != nil {
		return Credential{}, fmt.Errorf("platform credential exchange: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if err != nil {
		return Credential{}, fmt.Errorf("read Platform credential exchange: %w", err)
	}
	if len(responseBody) > responseLimit {
		return Credential{}, errors.New("platform credential response exceeds one MiB")
	}
	if response.StatusCode != http.StatusCreated {
		return Credential{}, exchangeError(response.StatusCode, responseBody)
	}

	var decoded struct {
		Data struct {
			Endpoint      string `json:"endpoint"`
			Username      string `json:"username"`
			Password      string `json:"password"`
			ExpiresAt     string `json:"expires_at"`
			Namespace     string `json:"namespace"`
			Principal     string `json:"principal"`
			PrincipalType string `json:"principal_type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return Credential{}, errors.New("platform credential response is invalid JSON")
	}
	dataPlaneURL, err := endpoint.Validate(decoded.Data.Endpoint, "Platform FerricStore endpoint")
	if err != nil {
		return Credential{}, err
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, decoded.Data.ExpiresAt)
	if err != nil || !expiresAt.After(time.Now()) {
		return Credential{}, errors.New("platform credential response has an invalid expiry")
	}
	if decoded.Data.Username == "" || decoded.Data.Password == "" {
		return Credential{}, errors.New("platform credential response is incomplete")
	}

	return Credential{
		Endpoint:      dataPlaneURL,
		Username:      decoded.Data.Username,
		Password:      decoded.Data.Password,
		ExpiresAt:     expiresAt,
		Namespace:     decoded.Data.Namespace,
		Principal:     decoded.Data.Principal,
		PrincipalType: decoded.Data.PrincipalType,
	}, nil
}

func validateControlURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("platform control URL is invalid")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("platform control URL must not contain credentials, query, or fragment")
	}
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !isLoopback(parsed.Hostname())) {
		return nil, errors.New("platform control URL must use HTTPS")
	}
	return parsed, nil
}

func exchangeError(status int, body []byte) error {
	var decoded struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &decoded)
	code := strings.TrimSpace(decoded.Error.Code)
	if code == "" {
		code = "request_failed"
	}
	return fmt.Errorf("platform credential exchange failed with status %d (%s)", status, code)
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
