package ferric

import (
	"context"
	"errors"

	"github.com/ferricstore/command-line/internal/connection"
	ferricstore "github.com/ferricstore/ferricstore-go"
)

// PasswordClientFactory constructs SDK clients that authenticate with OSS ACL credentials.
type PasswordClientFactory struct{}

// NewPasswordClient returns a lazily connected SDK client. The SDK authenticates
// every TCP connection it opens, including reconnects made during this process.
func (PasswordClientFactory) NewPasswordClient(rawURL, username, password string) (connection.Client, error) {
	return ferricstore.NewClientFromURL(
		rawURL,
		ferricstore.WithNativeOptions(
			ferricstore.WithNativeCredentials(username, password),
		),
	)
}

// PasswordValidator validates OSS credentials against a real FerricStore connection.
type PasswordValidator struct{}

// ValidatePassword authenticates and executes a PING.
func (PasswordValidator) ValidatePassword(ctx context.Context, rawURL, username, password string) error {
	client, err := (PasswordClientFactory{}).NewPasswordClient(rawURL, username, password)
	if err != nil {
		return err
	}
	_, pingErr := client.Ping(ctx)
	closeErr := client.Close()
	return errors.Join(pingErr, closeErr)
}
