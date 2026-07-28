package connection

import (
	"context"
	"errors"

	"github.com/ferricstore/command-line/internal/profile"
)

// PasswordClientFactory constructs a direct OSS username/password client.
type PasswordClientFactory interface {
	NewPasswordClient(string, string, string) (Client, error)
}

// PasswordProvider opens direct OSS connections from saved credentials.
type PasswordProvider struct {
	factory PasswordClientFactory
}

// NewPasswordProvider constructs the OSS connection provider.
func NewPasswordProvider(factory PasswordClientFactory) *PasswordProvider {
	return &PasswordProvider{factory: factory}
}

// Method returns the provider authentication method.
func (p *PasswordProvider) Method() profile.AuthMethod {
	return profile.AuthMethodPassword
}

// Open constructs a client configured to authenticate on its TCP connection.
func (p *PasswordProvider) Open(ctx context.Context, storedProfile profile.Profile, password string) (Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.factory == nil {
		return nil, errors.New("password connections are not configured")
	}
	if storedProfile.URL == "" {
		return nil, errors.New("profile has no FerricStore URL")
	}
	if storedProfile.Authentication.Username == "" {
		return nil, errors.New("profile has no username")
	}
	return p.factory.NewPasswordClient(
		storedProfile.URL,
		storedProfile.Authentication.Username,
		password,
	)
}
