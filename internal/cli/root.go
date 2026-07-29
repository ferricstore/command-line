// Package cli defines the ferric command-line interface.
package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ferricstore/command-line/internal/buildinfo"
	"github.com/ferricstore/command-line/internal/profile"
	"github.com/spf13/cobra"
)

const (
	profileFlagName    = "profile"
	defaultProfileName = "default"
	timeoutFlagName    = "timeout"
	outputFlagName     = "output"
)

// Execute runs the root command.
func Execute(info buildinfo.Info) error {
	return New(info).Execute()
}

// New constructs a command tree without global state.
func New(info buildinfo.Info, options ...Option) *cobra.Command {
	dependencies := defaultDependencies()
	for _, option := range options {
		if option != nil {
			option(&dependencies)
		}
	}
	dependencies.finalize()

	root := &cobra.Command{
		Use:   "ferric",
		Short: "Manage FerricStore from the command line",
		Long:  "Manage FerricStore data, FerricFlow queues and workflows, schedules, and operator services.",
		Example: "  ferric auth login --url ferric://127.0.0.1:6388\n" +
			"  ferric store get user:42\n" +
			"  ferric queue claim email --worker mailer-1\n" +
			"  ferric workflow describe order-42",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.PersistentFlags().String(profileFlagName, "", "saved connection profile")
	root.PersistentFlags().Lookup(profileFlagName).Hidden = true
	root.PersistentFlags().DurationVar(
		&dependencies.runtime.timeout,
		timeoutFlagName,
		dependencies.runtime.timeout,
		"maximum time for a network command",
	)
	root.PersistentFlags().Var(
		&dependencies.runtime.output,
		outputFlagName,
		"output format: auto, json, or raw",
	)
	mustRegisterProfileFlagCompletion(root, dependencies)

	root.AddCommand(newAuthCommand(dependencies))
	root.AddCommand(newProfileCommand(dependencies))
	root.AddCommand(newServerCommand(dependencies))
	root.AddCommand(newStoreCommand(dependencies))
	root.AddCommand(newQueueCommand(dependencies))
	root.AddCommand(newWorkflowCommand(dependencies))
	root.AddCommand(newClusterCommand(dependencies))
	root.AddCommand(newACLCommand(dependencies))
	root.AddCommand(newNamespaceCommand(dependencies))
	root.AddCommand(newQuotaCommand(dependencies))
	root.AddCommand(newPubSubCommand(dependencies))
	root.AddCommand(newVersionCommand(info))
	return root
}

func selectedProfile(command *cobra.Command, dependencies dependencies) (string, error) {
	if name, present, err := explicitProfile(command); present || err != nil {
		return name, err
	}
	if name := strings.TrimSpace(os.Getenv("FERRIC_PROFILE")); name != "" {
		return name, nil
	}
	if dependencies.profiles != nil {
		name, err := dependencies.profiles.Current(command.Context())
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, profile.ErrNotFound) {
			return "", fmt.Errorf("read current profile: %w", err)
		}
	}
	return defaultProfileName, nil
}

func explicitProfile(command *cobra.Command) (string, bool, error) {
	flag := command.Root().PersistentFlags().Lookup(profileFlagName)
	if flag != nil && flag.Changed {
		name, err := command.Root().PersistentFlags().GetString(profileFlagName)
		if err != nil {
			return "", true, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return "", true, errors.New("profile name is required")
		}
		return name, true, nil
	}
	return "", false, nil
}

func newVersionCommand(info buildinfo.Info) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print version information",
		Example: "  ferric version",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), info.String())
			return err
		},
	}
}
