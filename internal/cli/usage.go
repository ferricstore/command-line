package cli

import (
	"context"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	"github.com/spf13/cobra"
)

func newNamespaceCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "namespace",
		Short:   "Inspect namespace resource usage",
		Example: "  ferric namespace usage tenant-a:",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(newNamespaceUsageCommand(dependencies))
	return command
}

type namespaceUsageReader interface {
	NamespaceUsage(context.Context, string) (map[string]any, error)
}

func newNamespaceUsageCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "usage <prefix>",
		Short:   "Show resource usage for a key prefix",
		Example: "  ferric namespace usage tenant-a:",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "namespace usage", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[namespaceUsageReader](client, "namespace usage")
				if err != nil {
					return nil, err
				}
				return reader.NamespaceUsage(ctx, args[0])
			})
		},
	}
}

func newQuotaCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "quota",
		Short:   "Inspect namespace quota usage",
		Example: "  ferric quota usage tenant-a",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(newQuotaUsageCommand(dependencies))
	return command
}

type quotaUsageReader interface {
	QuotaUsage(context.Context, string) (any, error)
}

func newQuotaUsageCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "usage <namespace>",
		Short:   "Show quota consumption for a namespace",
		Example: "  ferric quota usage tenant-a",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "quota usage", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[quotaUsageReader](client, "quota usage")
				if err != nil {
					return nil, err
				}
				return reader.QuotaUsage(ctx, args[0])
			})
		},
	}
}
