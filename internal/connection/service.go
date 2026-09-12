// Package connection resolves saved profiles into authenticated FerricStore clients.
package connection

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ferricstore/command-line/internal/credential"
	"github.com/ferricstore/command-line/internal/profile"
)

// Client is the connection capability currently required by CLI commands.
type Client interface {
	Ping(context.Context, ...string) (string, error)
	Close() error
}

// Provider converts one saved authentication method into a cluster client.
// Enterprise implementations may exchange the saved secret for a temporary
// cluster credential before constructing the client.
type Provider interface {
	Method() profile.AuthMethod
	Open(context.Context, profile.Profile, string) (Client, error)
}

// Service routes saved or ephemeral credentials to connection providers.
type Service struct {
	profiles    profile.Store
	credentials credential.Store
	providers   map[profile.AuthMethod]Provider
}

// NewService constructs a saved-profile connection service.
func NewService(profiles profile.Store, credentials credential.Store, providers ...Provider) *Service {
	registered := make(map[profile.AuthMethod]Provider, len(providers))
	for _, provider := range providers {
		if provider != nil {
			registered[provider.Method()] = provider
		}
	}
	return &Service{profiles: profiles, credentials: credentials, providers: registered}
}

// Open resolves a named profile and returns a client owned by the caller.
func (s *Service) Open(ctx context.Context, profileName string) (Client, profile.Profile, error) {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return nil, profile.Profile{}, errors.New("profile name is required")
	}
	if s.profiles == nil || s.credentials == nil {
		return nil, profile.Profile{}, errors.New("saved-profile connections are not configured")
	}

	storedProfile, err := s.profiles.Get(ctx, profileName)
	if err != nil {
		return nil, profile.Profile{}, fmt.Errorf("load profile %q: %w", profileName, err)
	}
	provider, err := s.provider(storedProfile.Authentication.Method)
	if err != nil {
		return nil, profile.Profile{}, fmt.Errorf(
			"authentication method %q for profile %q is not available in this build",
			storedProfile.Authentication.Method,
			profileName,
		)
	}
	secret, err := s.credentials.Get(ctx, storedProfile.CredentialReference())
	if err != nil {
		return nil, profile.Profile{}, fmt.Errorf("load credential for profile %q: %w", profileName, err)
	}
	client, err := provider.Open(ctx, storedProfile, secret)
	if err != nil {
		return nil, profile.Profile{}, fmt.Errorf("connect using profile %q: %w", profileName, err)
	}
	if client == nil {
		return nil, profile.Profile{}, fmt.Errorf("connect using profile %q: provider returned no client", profileName)
	}
	return client, storedProfile, nil
}

// OpenEphemeral opens a client from credentials that are never persisted.
func (s *Service) OpenEphemeral(ctx context.Context, credentials EphemeralCredentials) (Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	provider, err := s.provider(credentials.Profile.Authentication.Method)
	if err != nil {
		return nil, err
	}
	client, err := provider.Open(ctx, credentials.Profile, credentials.Secret)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("authentication provider returned no client")
	}
	return client, nil
}

func (s *Service) provider(method profile.AuthMethod) (Provider, error) {
	provider, ok := s.providers[method]
	if !ok {
		return nil, fmt.Errorf("authentication method %q is not available in this build", method)
	}
	return provider, nil
}
