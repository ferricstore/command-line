package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/ferricstore/command-line/internal/profile"
)

// EnterpriseTokenValidator verifies a Platform token through a real exchange
// and data-plane authentication attempt.
type EnterpriseTokenValidator interface {
	ValidateEnterpriseToken(context.Context, profile.Profile, string) (string, error)
}

// EnterpriseTokenProvider validates human or service Platform tokens.
type EnterpriseTokenProvider struct {
	method    profile.AuthMethod
	validator EnterpriseTokenValidator
}

// NewEnterpriseTokenProvider constructs a provider for enterprise-sso or
// enterprise-api-token profiles.
func NewEnterpriseTokenProvider(
	method profile.AuthMethod,
	validator EnterpriseTokenValidator,
) *EnterpriseTokenProvider {
	return &EnterpriseTokenProvider{method: method, validator: validator}
}

// Method returns the configured authentication method.
func (p *EnterpriseTokenProvider) Method() profile.AuthMethod { return p.method }

// Login validates a renewable control-plane token and returns it for keyring storage.
func (p *EnterpriseTokenProvider) Login(
	ctx context.Context,
	request LoginRequest,
) (ProviderResult, error) {
	if p == nil || p.validator == nil {
		return ProviderResult{}, errors.New("enterprise authentication is not configured")
	}
	if p.method != profile.AuthMethodEnterpriseSSO && p.method != profile.AuthMethodEnterpriseAPIToken {
		return ProviderResult{}, errors.New("enterprise authentication method is invalid")
	}
	if request.Method != p.method {
		return ProviderResult{}, errors.New("enterprise authentication method does not match provider")
	}
	storedProfile := profile.Profile{
		Name:           request.ProfileName,
		ControlURL:     strings.TrimSpace(request.ControlURL),
		Organization:   strings.TrimSpace(request.Organization),
		Cluster:        strings.TrimSpace(request.Cluster),
		Authentication: profile.Authentication{Method: p.method},
	}
	if storedProfile.ControlURL == "" || storedProfile.Cluster == "" {
		return ProviderResult{}, errors.New("platform control URL and cluster ID are required")
	}
	if strings.TrimSpace(request.Secret) == "" {
		return ProviderResult{}, errors.New("platform token is required")
	}
	principal, err := p.validator.ValidateEnterpriseToken(ctx, storedProfile, request.Secret)
	if err != nil {
		return ProviderResult{}, err
	}
	return ProviderResult{
		Profile:   storedProfile,
		Principal: principal,
		Secret:    request.Secret,
	}, nil
}
