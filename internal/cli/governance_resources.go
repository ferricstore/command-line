package cli

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	ferricstore "github.com/ferricstore/ferricstore-go"
	"github.com/spf13/cobra"
)

func newBudgetCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "budget",
		Short: "Reserve and settle governed budgets",
		Example: "  ferric workflow governance budget get llm:tenant-a\n" +
			"  ferric workflow governance budget reserve llm:tenant-a 100 --limit 10000 --window 1h",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newBudgetGetCommand(dependencies),
		newBudgetListCommand(dependencies),
		newBudgetReserveCommand(dependencies),
		newBudgetCommitCommand(dependencies),
		newBudgetReleaseCommand(dependencies),
	)
	return command
}

type budgetGetter interface {
	BudgetGet(context.Context, string) (*ferricstore.BudgetResult, error)
}

func newBudgetGetCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "get <scope>",
		Aliases: []string{"describe", "inspect"},
		Short:   "Show a governed budget",
		Example: "  ferric workflow governance budget get llm:tenant-a",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "get budget", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				getter, err := requireClientCapability[budgetGetter](client, "budget reads")
				if err != nil {
					return nil, err
				}
				result, err := getter.BudgetGet(ctx, args[0])
				return compactOutput(result), err
			})
		},
	}
}

type budgetLister interface {
	BudgetList(context.Context, string, string, *int) ([]ferricstore.BudgetResult, error)
}

func newBudgetListCommand(dependencies dependencies) *cobra.Command {
	var scope string
	var partition string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List governed budgets",
		Example: "  ferric workflow governance budget list --scope llm:tenant-a\n" +
			"  ferric workflow governance budget list --partition tenant-a --limit 20",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if limit <= 0 {
				return errors.New("limit must be greater than zero")
			}
			if scope != "" && partition != "" {
				return errors.New("use --scope or --partition, not both")
			}
			return runNetworkCommand(command, dependencies, "list budgets", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				lister, err := requireClientCapability[budgetLister](client, "budget list")
				if err != nil {
					return nil, err
				}
				result, err := lister.BudgetList(ctx, scope, partition, ferricstore.Int(limit))
				return compactOutput(result), err
			})
		},
	}
	command.Flags().StringVar(&scope, "scope", "", "governance scope")
	command.Flags().StringVar(&partition, "partition", "", "partition key")
	command.Flags().IntVar(&limit, "limit", 100, "maximum budgets to return")
	return command
}

type budgetReserver interface {
	BudgetReserve(context.Context, string, int64, *int64, *int64, string, *int64) (ferricstore.BudgetResult, error)
}

func newBudgetReserveCommand(dependencies dependencies) *cobra.Command {
	var limit int64
	var window time.Duration
	var reservationID string
	command := &cobra.Command{
		Use:     "reserve <scope> <amount>",
		Short:   "Reserve governed budget capacity",
		Example: "  ferric workflow governance budget reserve llm:tenant-a 100 --limit 10000 --window 1h --reservation-id request-42",
		Args:    cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			amount, err := positiveInt64Argument("amount", args[1])
			if err != nil {
				return err
			}
			var limitPointer *int64
			if command.Flags().Changed("limit") {
				if limit <= 0 {
					return errors.New("limit must be greater than zero")
				}
				limitPointer = &limit
			}
			var windowPointer *int64
			if command.Flags().Changed("window") {
				value, err := positiveMilliseconds("window", window)
				if err != nil {
					return err
				}
				windowPointer = &value
			}
			return runNetworkCommand(command, dependencies, "reserve budget", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reserver, err := requireClientCapability[budgetReserver](client, "budget reservation")
				if err != nil {
					return nil, err
				}
				result, err := reserver.BudgetReserve(ctx, args[0], amount, limitPointer, windowPointer, reservationID, nil)
				return compactOutput(result), err
			})
		},
	}
	command.Flags().Int64Var(&limit, "limit", 0, "budget limit for the window")
	command.Flags().DurationVar(&window, "window", 0, "budget window")
	command.Flags().StringVar(&reservationID, "reservation-id", "", "stable reservation ID")
	return command
}

type budgetCommitter interface {
	BudgetCommit(context.Context, string, string, int64, map[string]any, *int64) (ferricstore.BudgetResult, error)
}

func newBudgetCommitCommand(dependencies dependencies) *cobra.Command {
	var input valueInput
	command := &cobra.Command{
		Use:     "commit <scope> <reservation-id> <actual-amount> [usage]",
		Short:   "Commit actual usage against a reservation",
		Example: "  ferric workflow governance budget commit llm:tenant-a request-42 87 '{\"tokens\":8700}' --json",
		Args:    cobra.RangeArgs(3, 4),
		RunE: func(command *cobra.Command, args []string) error {
			amount, err := nonNegativeInt64Argument("actual-amount", args[2])
			if err != nil {
				return err
			}
			var positional *string
			if len(args) == 4 {
				positional = &args[3]
			}
			usageValue, present, err := input.read(command, positional)
			if err != nil {
				return err
			}
			var usage map[string]any
			if present {
				var ok bool
				usage, ok = usageValue.(map[string]any)
				if !ok {
					return errors.New("usage must be a JSON object; add --json")
				}
			}
			return runNetworkCommand(command, dependencies, "commit budget", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				committer, err := requireClientCapability[budgetCommitter](client, "budget commit")
				if err != nil {
					return nil, err
				}
				result, err := committer.BudgetCommit(ctx, args[0], args[1], amount, usage, nil)
				return compactOutput(result), err
			})
		},
	}
	input.addFlags(command, "usage")
	return command
}

type budgetReleaser interface {
	BudgetRelease(context.Context, string, string, *int64) (ferricstore.BudgetResult, error)
}

func newBudgetReleaseCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "release <scope> <reservation-id>",
		Short:   "Release an unused budget reservation",
		Example: "  ferric workflow governance budget release llm:tenant-a request-42",
		Args:    cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "release budget", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				releaser, err := requireClientCapability[budgetReleaser](client, "budget release")
				if err != nil {
					return nil, err
				}
				result, err := releaser.BudgetRelease(ctx, args[0], args[1], nil)
				return compactOutput(result), err
			})
		},
	}
}

func newLimitCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "limit",
		Short: "Lease and spend distributed capacity limits",
		Example: "  ferric workflow governance limit get outbound-email\n" +
			"  ferric workflow governance limit lease outbound-email --shard 2 --amount 100 --ttl 1m",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newLimitGetCommand(dependencies),
		newLimitListCommand(dependencies),
		newLimitLeaseCommand(dependencies),
		newLimitSpendCommand(dependencies),
		newLimitReleaseCommand(dependencies),
	)
	return command
}

type limitGetter interface {
	LimitGet(context.Context, string, *int64) (*ferricstore.LimitResult, error)
}

func newLimitGetCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "get <scope>",
		Aliases: []string{"describe", "inspect"},
		Short:   "Show a distributed limit",
		Example: "  ferric workflow governance limit get outbound-email",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "get distributed limit", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				getter, err := requireClientCapability[limitGetter](client, "distributed limit reads")
				if err != nil {
					return nil, err
				}
				result, err := getter.LimitGet(ctx, args[0], nil)
				return compactOutput(result), err
			})
		},
	}
}

type limitLister interface {
	LimitList(context.Context, string, string, *int, *int64) ([]ferricstore.LimitResult, error)
}

func newLimitListCommand(dependencies dependencies) *cobra.Command {
	var scope string
	var partition string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List distributed limits",
		Example: "  ferric workflow governance limit list --scope outbound-email\n" +
			"  ferric workflow governance limit list --partition tenant-a --limit 20",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if limit <= 0 {
				return errors.New("limit must be greater than zero")
			}
			if scope != "" && partition != "" {
				return errors.New("use --scope or --partition, not both")
			}
			return runNetworkCommand(command, dependencies, "list distributed limits", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				lister, err := requireClientCapability[limitLister](client, "distributed limit list")
				if err != nil {
					return nil, err
				}
				result, err := lister.LimitList(ctx, scope, partition, ferricstore.Int(limit), nil)
				return compactOutput(result), err
			})
		},
	}
	command.Flags().StringVar(&scope, "scope", "", "governance scope")
	command.Flags().StringVar(&partition, "partition", "", "partition key")
	command.Flags().IntVar(&limit, "limit", 100, "maximum limits to return")
	return command
}

type limitLeaser interface {
	LimitLease(context.Context, string, int64, int64, int64, *int64, *int64) (ferricstore.LimitResult, error)
}

func newLimitLeaseCommand(dependencies dependencies) *cobra.Command {
	var shardID int64
	var amount int64
	var ttl time.Duration
	var limit int64
	command := &cobra.Command{
		Use:     "lease <scope>",
		Short:   "Lease distributed capacity to a shard",
		Example: "  ferric workflow governance limit lease outbound-email --shard 2 --amount 100 --ttl 1m --limit 1000",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if shardID < 0 {
				return errors.New("shard must not be negative")
			}
			if amount <= 0 {
				return errors.New("amount must be greater than zero")
			}
			ttlMS, err := positiveMilliseconds("ttl", ttl)
			if err != nil {
				return err
			}
			var limitPointer *int64
			if command.Flags().Changed("limit") {
				if limit <= 0 {
					return errors.New("limit must be greater than zero")
				}
				limitPointer = &limit
			}
			return runNetworkCommand(command, dependencies, "lease distributed capacity", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				leaser, err := requireClientCapability[limitLeaser](client, "distributed limit lease")
				if err != nil {
					return nil, err
				}
				result, err := leaser.LimitLease(ctx, args[0], shardID, amount, ttlMS, limitPointer, nil)
				return compactOutput(result), err
			})
		},
	}
	command.Flags().Int64Var(&shardID, "shard", 0, "shard ID")
	command.Flags().Int64Var(&amount, "amount", 0, "capacity to lease (required)")
	command.Flags().DurationVar(&ttl, "ttl", 0, "lease lifetime (required)")
	command.Flags().Int64Var(&limit, "limit", 0, "global capacity limit")
	return command
}

type limitSpender interface {
	LimitSpend(context.Context, string, int64, int64, *int64) (ferricstore.LimitResult, error)
}

func newLimitSpendCommand(dependencies dependencies) *cobra.Command {
	var shardID int64
	var amount int64
	command := &cobra.Command{
		Use:     "spend <scope>",
		Short:   "Spend capacity from a shard lease",
		Example: "  ferric workflow governance limit spend outbound-email --shard 2 --amount 1",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if shardID < 0 {
				return errors.New("shard must not be negative")
			}
			if amount <= 0 {
				return errors.New("amount must be greater than zero")
			}
			return runNetworkCommand(command, dependencies, "spend distributed capacity", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				spender, err := requireClientCapability[limitSpender](client, "distributed limit spend")
				if err != nil {
					return nil, err
				}
				result, err := spender.LimitSpend(ctx, args[0], shardID, amount, nil)
				return compactOutput(result), err
			})
		},
	}
	command.Flags().Int64Var(&shardID, "shard", 0, "shard ID")
	command.Flags().Int64Var(&amount, "amount", 0, "capacity to spend (required)")
	return command
}

type limitReleaser interface {
	LimitRelease(context.Context, string, ferricstore.LimitReleaseOptions) (ferricstore.LimitResult, error)
}

func newLimitReleaseCommand(dependencies dependencies) *cobra.Command {
	var shardID int64
	var reservationIDs []string
	command := &cobra.Command{
		Use:     "release <scope>",
		Short:   "Release exact distributed-limit reservations",
		Example: "  ferric workflow governance limit release outbound-email --shard 2 --reservation-id reservation-42",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if shardID < 0 {
				return errors.New("shard must not be negative")
			}
			if len(reservationIDs) == 0 {
				return errors.New("at least one --reservation-id is required")
			}
			options := ferricstore.LimitReleaseOptions{ShardID: shardID, ReservationIDs: reservationIDs}
			return runNetworkCommand(command, dependencies, "release distributed reservations", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				releaser, err := requireClientCapability[limitReleaser](client, "distributed limit release")
				if err != nil {
					return nil, err
				}
				result, err := releaser.LimitRelease(ctx, args[0], options)
				return compactOutput(result), err
			})
		},
	}
	command.Flags().Int64Var(&shardID, "shard", 0, "shard ID")
	command.Flags().StringSliceVar(&reservationIDs, "reservation-id", nil, "reservation ID returned by spend; repeatable")
	return command
}

func positiveInt64Argument(name, value string) (int64, error) {
	parsed, err := nonNegativeInt64Argument(name, value)
	if err != nil {
		return 0, err
	}
	if parsed == 0 {
		return 0, errors.New(name + " must be greater than zero")
	}
	return parsed, nil
}

func nonNegativeInt64Argument(name, value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, errors.New(name + " must be a non-negative integer")
	}
	return parsed, nil
}

func newEffectCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "effect",
		Short: "Reserve and settle governed side effects",
		Example: "  ferric workflow governance effect get order-42 charge-card\n" +
			"  ferric workflow governance effect reserve order-42 charge-card payment --lease-token TOKEN --fencing-token 1",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newEffectGetCommand(dependencies),
		newEffectReserveCommand(dependencies),
		newEffectStatusCommand(dependencies, "confirm"),
		newEffectStatusCommand(dependencies, "fail"),
		newEffectStatusCommand(dependencies, "compensate"),
	)
	return command
}

type effectGetter interface {
	EffectGet(context.Context, string, string, string) (*ferricstore.EffectResult, error)
}

func newEffectGetCommand(dependencies dependencies) *cobra.Command {
	var partition string
	command := &cobra.Command{
		Use:     "get <flow-id> <effect-key>",
		Aliases: []string{"describe", "inspect"},
		Short:   "Show a governed effect",
		Example: "  ferric workflow governance effect get order-42 charge-card --partition tenant-a",
		Args:    cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "get governed effect", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				getter, err := requireClientCapability[effectGetter](client, "effect reads")
				if err != nil {
					return nil, err
				}
				result, err := getter.EffectGet(ctx, args[0], args[1], partition)
				return compactOutput(result), err
			})
		},
	}
	command.Flags().StringVar(&partition, "partition", "", "partition key")
	return command
}

type effectReserver interface {
	EffectReserve(context.Context, string, string, string, ferricstore.EffectReserveOptions) (ferricstore.EffectResult, error)
}

func newEffectReserveCommand(dependencies dependencies) *cobra.Command {
	var leaseFlags flowLeaseFlags
	var digest string
	var idempotencyKey string
	var scope string
	command := &cobra.Command{
		Use:     "reserve <flow-id> <effect-key> <effect-type>",
		Short:   "Reserve a governed side effect",
		Example: "  ferric workflow governance effect reserve order-42 charge-card payment --lease-token TOKEN --fencing-token 1 --operation-digest sha256:abc --idempotency-key charge-42",
		Args:    cobra.ExactArgs(3),
		RunE: func(command *cobra.Command, args []string) error {
			if err := leaseFlags.validate(true, false); err != nil {
				return err
			}
			if strings.TrimSpace(digest) == "" {
				return errors.New("operation-digest is required; use --operation-digest <digest>")
			}
			options := ferricstore.EffectReserveOptions{
				PartitionKey: leaseFlags.partition, LeaseToken: leaseFlags.leaseToken,
				FencingToken: ferricstore.Int64(leaseFlags.fencingToken), OperationDigest: digest,
				IdempotencyKey: idempotencyKey, GovernanceScope: scope,
			}
			return runNetworkCommand(command, dependencies, "reserve governed effect", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reserver, err := requireClientCapability[effectReserver](client, "effect reservation")
				if err != nil {
					return nil, err
				}
				result, err := reserver.EffectReserve(ctx, args[0], args[1], args[2], options)
				return compactOutput(result), err
			})
		},
	}
	leaseFlags.add(command, true, false)
	command.Flags().StringVar(&digest, "operation-digest", "", "stable digest of the external operation")
	command.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "external idempotency key")
	command.Flags().StringVar(&scope, "scope", "", "governance scope")
	return command
}

type effectConfirmer interface {
	EffectConfirm(context.Context, string, string, ferricstore.EffectStatusOptions) (ferricstore.EffectResult, error)
}

type effectFailer interface {
	EffectFail(context.Context, string, string, ferricstore.EffectStatusOptions) (ferricstore.EffectResult, error)
}

type effectCompensator interface {
	EffectCompensate(context.Context, string, string, ferricstore.EffectStatusOptions) (ferricstore.EffectResult, error)
}

func newEffectStatusCommand(dependencies dependencies, action string) *cobra.Command {
	var leaseFlags flowLeaseFlags
	var externalID string
	var errorMessage string
	var reason string
	var latency time.Duration
	command := &cobra.Command{
		Use:     action + " <flow-id> <effect-key>",
		Short:   strings.ToUpper(action[:1]) + action[1:] + " a governed side effect",
		Example: "  ferric workflow governance effect " + action + " order-42 charge-card --lease-token TOKEN --fencing-token 1" + effectActionExampleSuffix(action),
		Args:    cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			if err := leaseFlags.validate(true, false); err != nil {
				return err
			}
			options := ferricstore.EffectStatusOptions{
				PartitionKey: leaseFlags.partition, LeaseToken: leaseFlags.leaseToken,
				FencingToken: ferricstore.Int64(leaseFlags.fencingToken), ExternalID: externalID,
				Error: errorMessage, Reason: reason,
			}
			if command.Flags().Changed("latency") {
				value, err := positiveMilliseconds("latency", latency)
				if err != nil {
					return err
				}
				options.LatencyMS = &value
			}
			return runNetworkCommand(command, dependencies, action+" governed effect", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				switch action {
				case "confirm":
					operator, err := requireClientCapability[effectConfirmer](client, "effect confirmation")
					if err != nil {
						return nil, err
					}
					result, err := operator.EffectConfirm(ctx, args[0], args[1], options)
					return compactOutput(result), err
				case "fail":
					operator, err := requireClientCapability[effectFailer](client, "effect failure")
					if err != nil {
						return nil, err
					}
					result, err := operator.EffectFail(ctx, args[0], args[1], options)
					return compactOutput(result), err
				default:
					operator, err := requireClientCapability[effectCompensator](client, "effect compensation")
					if err != nil {
						return nil, err
					}
					result, err := operator.EffectCompensate(ctx, args[0], args[1], options)
					return compactOutput(result), err
				}
			})
		},
	}
	leaseFlags.add(command, true, false)
	command.Flags().StringVar(&externalID, "external-id", "", "external operation ID")
	command.Flags().StringVar(&errorMessage, "error", "", "external failure message")
	command.Flags().StringVar(&reason, "reason", "", "status reason")
	command.Flags().DurationVar(&latency, "latency", 0, "external operation latency")
	return command
}

func effectActionExampleSuffix(action string) string {
	switch action {
	case "confirm":
		return " --external-id payment-123 --latency 250ms"
	case "fail":
		return " --error timeout"
	default:
		return " --reason refunded"
	}
}
