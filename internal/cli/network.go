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
	profileName, err := selectedProfile(command, dependencies)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(command.Context(), timeout)
	defer cancel()
	client, storedProfile, err := dependencies.connections.Open(ctx, profileName)
	if err != nil {
		return err
	}
	result, operationErr := operation(ctx, client, storedProfile)
	closeErr := client.Close()
	if err := errors.Join(operationErr, closeErr); err != nil {
		return fmt.Errorf("%s using profile %q: %w", name, profileName, err)
	}
	return writeResult(command.OutOrStdout(), format, result)
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
