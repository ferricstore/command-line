// Package auth coordinates login providers and persisted login state.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	ProfileName  string
	Method       profile.AuthMethod
	URL          string
	ControlURL   string
	Organization string
	Cluster      string
	Username     string
	Secret       string
	Store        bool
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
	Warning   error
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
	mutation    chan struct{}
}

// NewService constructs a login service.
func NewService(profiles profile.Store, credentials credential.Store, providers ...Provider) *Service {
	registered := make(map[profile.AuthMethod]Provider, len(providers))
	for _, provider := range providers {
		if provider != nil {
			registered[provider.Method()] = provider
		}
	}
	mutation := make(chan struct{}, 1)
	mutation <- struct{}{}
	return &Service{profiles: profiles, credentials: credentials, providers: registered, mutation: mutation}
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
	if providerResult.Profile.Authentication.Method != request.Method || provider.Method() != request.Method {
		return LoginResult{}, errors.New("authentication provider returned a mismatched authentication method")
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
	credentialReference, err := newCredentialReference()
	if err != nil {
		return LoginResult{}, err
	}
	persistedProfile := providerResult.Profile
	persistedProfile.Authentication.CredentialRef = credentialReference

	err = s.withCredentialMutation(ctx, func(mutationContext context.Context) error {
		previousReference := request.ProfileName
		previousProfile, previousErr := s.profiles.Get(mutationContext, request.ProfileName)
		if previousErr == nil {
			previousReference = previousProfile.CredentialReference()
		} else if !errors.Is(previousErr, profile.ErrNotFound) {
			return previousErr
		}
		if err := s.credentials.Put(mutationContext, credentialReference, providerResult.Secret); err != nil {
			return err
		}
		if err := s.profiles.Put(mutationContext, persistedProfile); err != nil {
			if rollbackErr := deleteCredentialForRollback(mutationContext, s.credentials, credentialReference); rollbackErr != nil {
				return errors.Join(err, fmt.Errorf("rollback credential for profile %q: %w", request.ProfileName, rollbackErr))
			}
			return err
		}
		if previousReference != credentialReference {
			if cleanupErr := s.credentials.Delete(mutationContext, previousReference); cleanupErr != nil && !errors.Is(cleanupErr, credential.ErrNotFound) {
				result.Warning = fmt.Errorf("remove previous credential for profile %q: %w", request.ProfileName, cleanupErr)
			}
		}
		return nil
	})
	if err != nil {
		return LoginResult{}, err
	}
	result.Profile = persistedProfile
	result.Stored = true
	return result, nil
}

func (s *Service) withCredentialMutation(ctx context.Context, operation func(context.Context) error) error {
	if coordinator, ok := s.profiles.(profile.CredentialMutationCoordinator); ok {
		return coordinator.WithCredentialMutation(ctx, operation)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.mutation:
	}
	defer func() { s.mutation <- struct{}{} }()
	return operation(ctx)
}

func deleteCredentialForRollback(ctx context.Context, store credential.Store, reference string) error {
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), credentialRollbackTimeout)
	defer cancel()
	if err := store.Delete(cleanupContext, reference); !errors.Is(err, credential.ErrNotFound) {
		return err
	}
	return nil
}

func newCredentialReference() (string, error) {
	var generation [16]byte
	if _, err := rand.Read(generation[:]); err != nil {
		return "", fmt.Errorf("generate credential reference: %w", err)
	}
	return "credential-" + hex.EncodeToString(generation[:]), nil
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
	if s.profiles == nil {
		return LogoutResult{}, errors.New("profile persistence is not configured")
	}
	removed := false
	err := s.withCredentialMutation(ctx, func(mutationContext context.Context) error {
		reference := profileName
		storedProfile, profileErr := s.profiles.Get(mutationContext, profileName)
		if profileErr == nil {
			reference = storedProfile.CredentialReference()
		} else if !errors.Is(profileErr, profile.ErrNotFound) {
			return fmt.Errorf("load profile %q: %w", profileName, profileErr)
		}
		if err := s.credentials.Delete(mutationContext, reference); errors.Is(err, credential.ErrNotFound) {
			return nil
		} else if err != nil {
			return fmt.Errorf("delete credential for profile %q: %w", profileName, err)
		}
		removed = true
		return nil
	})
	if err != nil {
		return LogoutResult{}, err
	}
	return LogoutResult{Removed: removed}, nil
}
