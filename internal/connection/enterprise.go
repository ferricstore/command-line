package connection

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ferricstore/command-line/internal/platformapi"
	"github.com/ferricstore/command-line/internal/profile"
)

// EnterpriseCredentialBroker exchanges a renewable Platform token for a
// temporary data-plane credential.
type EnterpriseCredentialBroker interface {
	Exchange(context.Context, string, string, string, string, time.Duration) (platformapi.Credential, error)
}

// EnterpriseTokenProvider opens native clients without forwarding the Platform token.
type EnterpriseTokenProvider struct {
	method  profile.AuthMethod
	broker  EnterpriseCredentialBroker
	factory PasswordClientFactory
	ttl     time.Duration
}

// NewEnterpriseTokenProvider constructs a human or service Platform provider.
func NewEnterpriseTokenProvider(
	method profile.AuthMethod,
	broker EnterpriseCredentialBroker,
	factory PasswordClientFactory,
) *EnterpriseTokenProvider {
	return &EnterpriseTokenProvider{method: method, broker: broker, factory: factory, ttl: 15 * time.Minute}
}

// Method returns the configured Enterprise authentication method.
func (p *EnterpriseTokenProvider) Method() profile.AuthMethod { return p.method }

// Open exchanges the saved control-plane token and creates a native SDK client.
func (p *EnterpriseTokenProvider) Open(
	ctx context.Context,
	storedProfile profile.Profile,
	token string,
) (Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p == nil || p.broker == nil || p.factory == nil {
		return nil, errors.New("Enterprise connections are not configured")
	}
	if p.method != profile.AuthMethodEnterpriseSSO && p.method != profile.AuthMethodEnterpriseAPIToken {
		return nil, errors.New("Enterprise authentication method is invalid")
	}
	if storedProfile.Authentication.Method != p.method {
		return nil, errors.New("profile authentication method does not match provider")
	}
	if strings.TrimSpace(storedProfile.ControlURL) == "" || strings.TrimSpace(storedProfile.Cluster) == "" {
		return nil, errors.New("profile requires Platform control URL and cluster ID")
	}
	credential, err := p.broker.Exchange(
		ctx,
		storedProfile.ControlURL,
		token,
		storedProfile.Organization,
		storedProfile.Cluster,
		p.ttl,
	)
	if err != nil {
		return nil, err
	}
	dataPlaneProfile := profile.Profile{
		URL:        credential.Endpoint,
		CACertFile: storedProfile.CACertFile,
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: credential.Username,
		},
	}
	return p.factory.NewPasswordClient(ctx, dataPlaneProfile, credential.Password)
}
