package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	"github.com/spf13/cobra"
)

type sdkCommander interface {
	Command(context.Context, ...any) (any, error)
}

func newConfirmedSDKCommand(dependencies dependencies, spec sdkCommandSpec) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:     spec.use,
		Aliases: spec.aliases,
		Short:   spec.short,
		Long:    spec.long,
		Example: spec.example,
		GroupID: spec.group,
		Args:    sdkCommandArgs(spec),
		RunE: func(command *cobra.Command, args []string) error {
			if !yes {
				return errors.New(spec.name + " requires --yes")
			}
			wireArgs := append([]any(nil), spec.wire...)
			for _, argument := range args {
				wireArgs = append(wireArgs, argument)
			}
			return runNetworkCommand(command, dependencies, spec.name, func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				commander, err := requireClientCapability[sdkCommander](client, "SDK commands")
				if err != nil {
					return nil, err
				}
				return commander.Command(ctx, wireArgs...)
			})
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "confirm the safety-sensitive operation")
	return command
}

type sdkCommandSpec struct {
	name      string
	aliases   []string
	use       string
	short     string
	long      string
	example   string
	group     string
	wire      []any
	minArgs   int
	maxArgs   int
	validator cobra.PositionalArgs
}

func newSDKCommand(dependencies dependencies, spec sdkCommandSpec) *cobra.Command {
	use := spec.use
	if use == "" {
		use = spec.name
	}
	command := &cobra.Command{
		Use:     use,
		Aliases: spec.aliases,
		Short:   spec.short,
		Long:    spec.long,
		Example: spec.example,
		GroupID: spec.group,
		Args:    sdkCommandArgs(spec),
		RunE: func(command *cobra.Command, args []string) error {
			wireArgs := append([]any(nil), spec.wire...)
			for _, argument := range args {
				wireArgs = append(wireArgs, argument)
			}
			return runNetworkCommand(command, dependencies, strings.ToLower(fmt.Sprint(spec.wire[0])), func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				commander, err := requireClientCapability[sdkCommander](client, "SDK commands")
				if err != nil {
					return nil, err
				}
				return commander.Command(ctx, wireArgs...)
			})
		},
	}
	// Stop parsing flags after the first protocol argument. This preserves
	// Redis-style values such as -1 and tokens such as WITHSCORES while keeping
	// `ferric store get --help` and inherited flags functional.
	command.Flags().SetInterspersed(false)
	return command
}

func sdkCommandArgs(spec sdkCommandSpec) cobra.PositionalArgs {
	if spec.validator != nil {
		return spec.validator
	}
	return func(command *cobra.Command, args []string) error {
		if len(args) < spec.minArgs {
			return fmt.Errorf("%s requires at least %d argument(s)", command.CommandPath(), spec.minArgs)
		}
		if spec.maxArgs >= 0 && len(args) > spec.maxArgs {
			return fmt.Errorf("%s accepts at most %d argument(s)", command.CommandPath(), spec.maxArgs)
		}
		return nil
	}
}
