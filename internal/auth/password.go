package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/ferricstore/command-line/internal/profile"
)

// PasswordValidator verifies OSS FerricStore username/password credentials.
type PasswordValidator interface {
	ValidatePassword(context.Context, string, string, string) error
}

// PasswordProvider performs direct OSS username/password login.
type PasswordProvider struct {
	validator PasswordValidator
}

// NewPasswordProvider constructs the OSS password provider.
func NewPasswordProvider(validator PasswordValidator) *PasswordProvider {
	return &PasswordProvider{validator: validator}
}

// Method returns the provider authentication method.
func (p *PasswordProvider) Method() profile.AuthMethod {
	return profile.AuthMethodPassword
}

// Login validates direct OSS credentials.
func (p *PasswordProvider) Login(ctx context.Context, request LoginRequest) (ProviderResult, error) {
	if p.validator == nil {
		return ProviderResult{}, errors.New("password authentication is not configured")
	}
	rawURL := strings.TrimSpace(request.URL)
	if rawURL == "" {
		return ProviderResult{}, errors.New("FerricStore URL is required")
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return ProviderResult{}, errors.New("FerricStore URL is invalid")
	}
	if parsedURL.User != nil {
		return ProviderResult{}, errors.New("FerricStore URL must not contain credentials; use --username and the password prompt")
	}
	username := strings.TrimSpace(request.Username)
	if username == "" {
		username = "default"
	}
	if err := p.validator.ValidatePassword(ctx, rawURL, username, request.Secret); err != nil {
		return ProviderResult{}, fmt.Errorf("login failed: %w", err)
	}
	return ProviderResult{
		Profile: profile.Profile{
			Name: request.ProfileName,
			URL:  rawURL,
			Authentication: profile.Authentication{
				Method:   profile.AuthMethodPassword,
				Username: username,
			},
		},
		Principal: username,
		Secret:    request.Secret,
	}, nil
}
