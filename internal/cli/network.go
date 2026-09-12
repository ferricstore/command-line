package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	"github.com/spf13/cobra"
)

type clientOperation func(context.Context, connection.Client, profile.Profile) (any, error)

type activeConnectionSource string

const (
	activeConnectionProfile     activeConnectionSource = "profile"
	activeConnectionEnvironment activeConnectionSource = "environment"
)

type activeConnection struct {
	client      connection.Client
	profile     profile.Profile
	profileName string
	source      activeConnectionSource
}

func runNetworkCommand(command *cobra.Command, dependencies dependencies, name string, operation clientOperation) error {
	if dependencies.connections == nil {
		return errors.New("connection service is not configured")
	}
	if operation == nil {
		return errors.New("network operation is not configured")
	}
	timeout := runtimeTimeout(dependencies)
	format := outputAuto
	if dependencies.runtime != nil {
		format = dependencies.runtime.output
	}
	if timeout <= 0 {
		return errors.New("timeout must be greater than zero")
	}
	ctx, cancel := context.WithTimeout(command.Context(), timeout)
	defer cancel()
	active, err := openActiveConnection(ctx, command, dependencies)
	if err != nil {
		return err
	}
	result, operationErr := operation(ctx, active.client, active.profile)
	closeErr := active.client.Close()
	if operationErr != nil {
		if err := errors.Join(operationErr, closeErr); err != nil {
			return fmt.Errorf("%s using %s: %w", name, active.description(), err)
		}
	}
	if err := writeResult(command.OutOrStdout(), format, result); err != nil {
		return fmt.Errorf("%s using %s: %w", name, active.description(), err)
	}
	if closeErr != nil {
		_, _ = fmt.Fprintf(
			command.ErrOrStderr(),
			"warning: close %s after successful %s: %v\n",
			active.description(),
			name,
			closeErr,
		)
	}
	return nil
}

const blockingResponseMargin = time.Second

func validateBlockingWait(dependencies dependencies, wait time.Duration, source string) error {
	if wait <= 0 {
		return nil
	}
	timeout := runtimeTimeout(dependencies)
	if timeout <= wait || timeout-wait < blockingResponseMargin {
		return fmt.Errorf(
			"--timeout (%s) must exceed %s (%s) by at least %s",
			timeout,
			source,
			wait,
			blockingResponseMargin,
		)
	}
	return nil
}

func openActiveConnection(
	ctx context.Context,
	command *cobra.Command,
	dependencies dependencies,
) (activeConnection, error) {
	profileName, explicit, err := explicitProfile(command)
	if err != nil {
		return activeConnection{}, err
	}
	if explicit {
		return openSavedConnection(ctx, dependencies, profileName)
	}

	if dependencies.environment != nil {
		credentials, present, err := dependencies.environment.Resolve(ctx)
		if err != nil {
			return activeConnection{}, fmt.Errorf("resolve environment credentials: %w", err)
		}
		if present {
			client, err := dependencies.connections.OpenEphemeral(ctx, credentials)
			if err != nil {
				return activeConnection{}, fmt.Errorf("connect using environment credentials: %w", err)
			}
			return activeConnection{
				client:  client,
				profile: credentials.Profile,
				source:  activeConnectionEnvironment,
			}, nil
		}
	}

	profileName, err = selectedProfile(command, dependencies)
	if err != nil {
		return activeConnection{}, err
	}
	return openSavedConnection(ctx, dependencies, profileName)
}

func openSavedConnection(
	ctx context.Context,
	dependencies dependencies,
	profileName string,
) (activeConnection, error) {
	client, storedProfile, err := dependencies.connections.Open(ctx, profileName)
	if err != nil {
		return activeConnection{}, err
	}
	return activeConnection{
		client:      client,
		profile:     storedProfile,
		profileName: profileName,
		source:      activeConnectionProfile,
	}, nil
}

func (c activeConnection) description() string {
	if c.source == activeConnectionEnvironment {
		return "environment credentials"
	}
	return fmt.Sprintf("profile %q", c.profileName)
}

func runtimeTimeout(dependencies dependencies) time.Duration {
	if dependencies.runtime != nil {
		return dependencies.runtime.timeout
	}
	return 10 * time.Second
}

func runtimeOutput(dependencies dependencies) outputFormat {
	if dependencies.runtime != nil {
		return dependencies.runtime.output
	}
	return outputAuto
}

func requireClientCapability[T any](client connection.Client, name string) (T, error) {
	capability, ok := client.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("the active SDK client does not support %s", name)
	}
	return capability, nil
}
