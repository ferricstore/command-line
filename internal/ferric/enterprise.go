package ferric

import (
	"context"
	"errors"
	"time"

	"github.com/ferricstore/command-line/internal/platformapi"
	"github.com/ferricstore/command-line/internal/profile"
)

// EnterpriseTokenValidator validates Platform tokens by exchanging them for a
// temporary native credential and executing PING with that credential.
type EnterpriseTokenValidator struct {
	Broker *platformapi.Client
}

// ValidateEnterpriseToken performs the complete control-plane-to-data-plane path.
func (v EnterpriseTokenValidator) ValidateEnterpriseToken(
	ctx context.Context,
	storedProfile profile.Profile,
	token string,
) (string, error) {
	if v.Broker == nil {
		return "", errors.New("platform API client is not configured")
	}
	credential, err := v.Broker.Exchange(
		ctx,
		storedProfile.ControlURL,
		token,
		storedProfile.Organization,
		storedProfile.Cluster,
		15*time.Minute,
	)
	if err != nil {
		return "", err
	}
	dataPlaneProfile := profile.Profile{
		URL:        credential.Endpoint,
		CACertFile: storedProfile.CACertFile,
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: credential.Username,
		},
	}
	client, err := (PasswordClientFactory{}).NewPasswordClient(ctx, dataPlaneProfile, credential.Password)
	if err != nil {
		return "", err
	}
	_, pingErr := client.Ping(ctx)
	closeErr := client.Close()
	if err := errors.Join(pingErr, closeErr); err != nil {
		return "", err
	}
	if credential.Principal != "" {
		return credential.Principal, nil
	}
	return credential.Username, nil
}
