package cli

import (
	"time"

	"github.com/ferricstore/command-line/internal/auth"
	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/credential"
	"github.com/ferricstore/command-line/internal/ferric"
	"github.com/ferricstore/command-line/internal/platformapi"
	"github.com/ferricstore/command-line/internal/profile"
)

type dependencies struct {
	login          *auth.Service
	connections    *connection.Service
	profiles       profile.Manager
	credentials    credential.Store
	environment    connection.CredentialSource
	passwordReader passwordReader
	runtime        *runtimeOptions
}

type runtimeOptions struct {
	timeout time.Duration
	output  outputFormat
}

// Option customizes CLI dependencies.
type Option func(*dependencies)

// WithLoginService replaces the login service.
func WithLoginService(service *auth.Service) Option {
	return func(dependencies *dependencies) {
		dependencies.login = service
	}
}

// WithConnectionService replaces authenticated connection resolution.
func WithConnectionService(service *connection.Service) Option {
	return func(dependencies *dependencies) {
		dependencies.connections = service
	}
}

// WithProfileManager replaces profile management and default selection.
func WithProfileManager(manager profile.Manager) Option {
	return func(dependencies *dependencies) {
		dependencies.profiles = manager
	}
}

// WithCredentialStore replaces credential deletion used by profile management.
func WithCredentialStore(store credential.Store) Option {
	return func(dependencies *dependencies) {
		dependencies.credentials = store
	}
}

// WithEnvironmentCredentialSource replaces ephemeral environment resolution.
func WithEnvironmentCredentialSource(source connection.CredentialSource) Option {
	return func(dependencies *dependencies) {
		dependencies.environment = source
	}
}

// WithPasswordReader replaces interactive password input.
func WithPasswordReader(reader passwordReader) Option {
	return func(dependencies *dependencies) {
		dependencies.passwordReader = reader
	}
}

func defaultDependencies() dependencies {
	profiles := profile.NewDefaultFileStore()
	credentials := credential.NewKeyringStore()
	return dependencies{
		profiles:       profiles,
		credentials:    credentials,
		environment:    connection.NewEnvironmentCredentialSource(),
		passwordReader: terminalPasswordReader{},
		runtime: &runtimeOptions{
			timeout: 10 * time.Second,
			output:  outputAuto,
		},
	}
}

// finalize builds default services only after all dependency options have been
// applied. This keeps the profile and credential stores used by commands,
// authentication, and connections on one coherent dependency graph.
func (d *dependencies) finalize() {
	platformClient := platformapi.NewClient(nil)
	enterpriseValidator := ferric.EnterpriseTokenValidator{Broker: platformClient}

	if d.login == nil {
		d.login = auth.NewService(
			d.profiles,
			d.credentials,
			auth.NewPasswordProvider(ferric.PasswordValidator{}),
			auth.NewEnterpriseTokenProvider(profile.AuthMethodEnterpriseSSO, enterpriseValidator),
			auth.NewEnterpriseTokenProvider(
				profile.AuthMethodEnterpriseAPIToken,
				enterpriseValidator,
			),
		)
	}
	if d.connections == nil {
		d.connections = connection.NewService(
			d.profiles,
			d.credentials,
			connection.NewPasswordProvider(ferric.PasswordClientFactory{}),
			connection.NewEnterpriseTokenProvider(
				profile.AuthMethodEnterpriseSSO,
				platformClient,
				ferric.PasswordClientFactory{},
			),
			connection.NewEnterpriseTokenProvider(
				profile.AuthMethodEnterpriseAPIToken,
				platformClient,
				ferric.PasswordClientFactory{},
			),
		)
	}
}
