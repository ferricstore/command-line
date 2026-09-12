package cli

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ferricstore/command-line/internal/commandsafety"
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
			if err := validateSDKBlockingWait(dependencies, spec, args); err != nil {
				return err
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
	// Once the first protocol argument is seen, preserve every remaining token
	// exactly, including negative numbers and values beginning with a dash.
	command.Flags().SetInterspersed(false)
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
	blocking  blockingWaitParser
}

func newSDKCommand(dependencies dependencies, spec sdkCommandSpec) *cobra.Command {
	if commandsafety.RequiresConfirmation(spec.wire) {
		return newConfirmedSDKCommand(dependencies, spec)
	}
	return newUncheckedSDKCommand(dependencies, spec)
}

func newUncheckedSDKCommand(dependencies dependencies, spec sdkCommandSpec) *cobra.Command {
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
			if err := validateSDKBlockingWait(dependencies, spec, args); err != nil {
				return err
			}
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

type blockingWaitParser func([]string) (*time.Duration, error)

func validateSDKBlockingWait(dependencies dependencies, spec sdkCommandSpec, args []string) error {
	if spec.blocking == nil {
		return nil
	}
	wait, err := spec.blocking(args)
	if err != nil {
		return err
	}
	if wait == nil {
		return nil
	}
	if *wait == 0 {
		return errors.New("server wait is indefinite and cannot fit within --timeout")
	}
	return validateBlockingWait(dependencies, *wait, "server wait")
}

func secondsWaitAt(index int) blockingWaitParser {
	return func(args []string) (*time.Duration, error) {
		actualIndex := index
		if actualIndex < 0 {
			actualIndex = len(args) + actualIndex
		}
		if actualIndex < 0 || actualIndex >= len(args) {
			return nil, nil
		}
		seconds, err := strconv.ParseFloat(args[actualIndex], 64)
		maxSeconds := float64(time.Duration(math.MaxInt64)) / float64(time.Second)
		if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds >= maxSeconds {
			return nil, fmt.Errorf("invalid blocking timeout %q", args[actualIndex])
		}
		wait := time.Duration(seconds * float64(time.Second))
		return &wait, nil
	}
}

func millisecondsWaitAt(index int) blockingWaitParser {
	return func(args []string) (*time.Duration, error) {
		actualIndex := index
		if actualIndex < 0 {
			actualIndex = len(args) + actualIndex
		}
		if actualIndex < 0 || actualIndex >= len(args) {
			return nil, nil
		}
		milliseconds, err := strconv.ParseInt(args[actualIndex], 10, 64)
		maxMilliseconds := int64(time.Duration(math.MaxInt64) / time.Millisecond)
		if err != nil || milliseconds < 0 || milliseconds > maxMilliseconds {
			return nil, fmt.Errorf("invalid blocking timeout %q", args[actualIndex])
		}
		wait := time.Duration(milliseconds) * time.Millisecond
		return &wait, nil
	}
}

func millisecondsWaitAfter(token string) blockingWaitParser {
	return func(args []string) (*time.Duration, error) {
		for index := 0; index < len(args); index++ {
			if !strings.EqualFold(args[index], token) {
				continue
			}
			if index+1 >= len(args) {
				return nil, fmt.Errorf("%s requires a timeout value", token)
			}
			return millisecondsWaitAt(index + 1)(args)
		}
		return nil, nil
	}
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
