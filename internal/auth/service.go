// Package auth coordinates login providers and persisted login state.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ferricstore/command-line/internal/credential"
	"github.com/ferricstore/command-line/internal/profile"
)

const credentialRollbackTimeout = 5 * time.Second

// LoginRequest contains provider-neutral login input.
type LoginRequest struct {
	ProfileName string
	Method      profile.AuthMethod
	URL         string
	CACertFile  string
	Username    string
	Secret      string
	Store       bool
}

// ProviderResult contains validated profile and credential state.
type ProviderResult struct {
	Profile   profile.Profile
	Principal string
	Secret    string
}

// LoginResult describes a completed login.
type LoginResult struct {
	Profile   profile.Profile
	Principal string
	Stored    bool
}

// LogoutResult describes removal of a locally stored credential.
type LogoutResult struct {
	Removed bool
}

// Provider authenticates one login method.
type Provider interface {
	Method() profile.AuthMethod
	Login(context.Context, LoginRequest) (ProviderResult, error)
}

// Service routes login requests and persists successful state.
type Service struct {
	profiles    profile.Store
	credentials credential.Store
	providers   map[profile.AuthMethod]Provider
}

// NewService constructs a login service.
func NewService(profiles profile.Store, credentials credential.Store, providers ...Provider) *Service {
	registered := make(map[profile.AuthMethod]Provider, len(providers))
	for _, provider := range providers {
		if provider != nil {
			registered[provider.Method()] = provider
		}
	}
	return &Service{profiles: profiles, credentials: credentials, providers: registered}
}

// Login authenticates and optionally persists a profile and credential.
func (s *Service) Login(ctx context.Context, request LoginRequest) (LoginResult, error) {
	request.ProfileName = strings.TrimSpace(request.ProfileName)
	if request.ProfileName == "" {
		return LoginResult{}, errors.New("profile name is required")
	}
	if request.Method == "" {
		request.Method = profile.AuthMethodPassword
	}

	provider, ok := s.providers[request.Method]
	if !ok {
		return LoginResult{}, fmt.Errorf("authentication method %q is not available in this build", request.Method)
	}
	providerResult, err := provider.Login(ctx, request)
	if err != nil {
		return LoginResult{}, err
	}
	if providerResult.Profile.Name != request.ProfileName {
		return LoginResult{}, errors.New("authentication provider returned a mismatched profile")
	}
	result := LoginResult{
		Profile:   providerResult.Profile,
		Principal: providerResult.Principal,
		Stored:    false,
	}
	if !request.Store {
		return result, nil
	}
	if s.profiles == nil || s.credentials == nil {
		return LoginResult{}, errors.New("credential persistence is not configured")
	}

	previousSecret, previousErr := s.credentials.Get(ctx, request.ProfileName)
	if previousErr != nil && !errors.Is(previousErr, credential.ErrNotFound) {
		return LoginResult{}, previousErr
	}
	if err := s.credentials.Put(ctx, request.ProfileName, providerResult.Secret); err != nil {
		return LoginResult{}, err
	}
	if err := s.profiles.Put(ctx, providerResult.Profile); err != nil {
		if rollbackErr := rollbackCredential(ctx, s.credentials, request.ProfileName, previousSecret, previousErr == nil); rollbackErr != nil {
			return LoginResult{}, errors.Join(err, fmt.Errorf("rollback credential for profile %q: %w", request.ProfileName, rollbackErr))
		}
		return LoginResult{}, err
	}
	result.Stored = true
	return result, nil
}

func rollbackCredential(ctx context.Context, store credential.Store, profileName, previous string, hadPrevious bool) error {
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), credentialRollbackTimeout)
	defer cancel()
	if hadPrevious {
		return store.Put(cleanupContext, profileName, previous)
	}
	if err := store.Delete(cleanupContext, profileName); !errors.Is(err, credential.ErrNotFound) {
		return err
	}
	return nil
}

// Logout removes a profile's local credential while preserving its metadata.
func (s *Service) Logout(ctx context.Context, profileName string) (LogoutResult, error) {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return LogoutResult{}, errors.New("profile name is required")
	}
	if s.credentials == nil {
		return LogoutResult{}, errors.New("credential persistence is not configured")
	}
	if err := s.credentials.Delete(ctx, profileName); errors.Is(err, credential.ErrNotFound) {
		return LogoutResult{}, nil
	} else if err != nil {
		return LogoutResult{}, fmt.Errorf("delete credential for profile %q: %w", profileName, err)
	}
	return LogoutResult{Removed: true}, nil
}
