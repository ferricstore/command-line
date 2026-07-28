package cli

import (
	"time"

	"github.com/ferricstore/command-line/internal/auth"
	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/credential"
	"github.com/ferricstore/command-line/internal/ferric"
	"github.com/ferricstore/command-line/internal/profile"
)

type dependencies struct {
	login          *auth.Service
	connections    *connection.Service
	profiles       profile.Manager
	credentials    credential.Store
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

// WithConnectionService replaces saved-profile connection resolution.
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

// WithPasswordReader replaces interactive password input.
func WithPasswordReader(reader passwordReader) Option {
	return func(dependencies *dependencies) {
		dependencies.passwordReader = reader
	}
}

func defaultDependencies() dependencies {
	profiles := profile.NewDefaultFileStore()
	credentials := credential.NewKeyringStore()
	login := auth.NewService(
		profiles,
		credentials,
		auth.NewPasswordProvider(ferric.PasswordValidator{}),
	)
	connections := connection.NewService(
		profiles,
		credentials,
		connection.NewPasswordProvider(ferric.PasswordClientFactory{}),
	)
	return dependencies{
		login:          login,
		connections:    connections,
		profiles:       profiles,
		credentials:    credentials,
		passwordReader: terminalPasswordReader{},
		runtime: &runtimeOptions{
			timeout: 10 * time.Second,
			output:  outputAuto,
		},
	}
}
