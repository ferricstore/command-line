package cli

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/outputcontract"
	"github.com/ferricstore/command-line/internal/profile"
	ferricstore "github.com/ferricstore/ferricstore-go"
	"github.com/spf13/cobra"
)

func newQueueCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "queue",
		Short: "Send, receive, and manage FerricFlow jobs",
		Long: "Manage the job-oriented FerricFlow lifecycle. Claims return a lease token and fencing token; " +
			"both must be carried into completion, retry, and failure commands.",
		Example: "  ferric queue enqueue email email-42 '{\"to\":\"ada@example.com\"}' --json\n" +
			"  ferric queue claim email --worker mailer-1\n" +
			"  ferric queue complete email-42 --lease-token TOKEN --fencing-token 1",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newQueueEnqueueCommand(dependencies),
		newFlowCreateManyCommand(dependencies, "queue"),
		newQueueClaimCommand(dependencies),
		newQueueReclaimCommand(dependencies),
		newFlowDescribeCommand(dependencies, "queue"),
		newFlowListCommand(dependencies, "queue"),
		newQueueExtendCommand(dependencies),
		newFlowCompleteCommand(dependencies, "queue"),
		newFlowCompleteManyCommand(dependencies, "queue"),
		newFlowRetryCommand(dependencies, "queue"),
		newFlowRetryManyCommand(dependencies, "queue"),
		newFlowFailCommand(dependencies, "queue"),
		newFlowFailManyCommand(dependencies, "queue"),
		newFlowCancelCommand(dependencies, "queue"),
		newFlowCancelManyCommand(dependencies, "queue"),
		newFlowHistoryCommand(dependencies, "queue"),
		newFlowStatsCommand(dependencies, "queue"),
		newFlowPolicyCommand(dependencies, "queue"),
	)
	return command
}

type flowEnqueuer interface {
	Enqueue(context.Context, ferricstore.CreateOptions) (*ferricstore.FlowRecord, error)
}

func newQueueEnqueueCommand(dependencies dependencies) *cobra.Command {
	var flags flowCreateFlags
	command := &cobra.Command{
		Use:     "enqueue <type> <id> [payload]",
		Aliases: []string{"send"},
		Short:   "Add a job to a queue",
		Long:    "Add a FerricFlow job. The type identifies the queue and the ID must be stable and unique.",
		Example: "  ferric queue enqueue email email-42 '{\"to\":\"ada@example.com\"}' --json\n" +
			"  ferric queue send report report-42 --file request.json --json --delay 5m",
		Args: cobra.RangeArgs(2, 3),
		RunE: func(command *cobra.Command, args []string) error {
			var payload *string
			if len(args) == 3 {
				payload = &args[2]
			}
			options, err := flags.options(command, args[0], args[1], payload)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "enqueue job", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				enqueuer, err := requireClientCapability[flowEnqueuer](client, "FerricFlow enqueue")
				if err != nil {
					return nil, err
				}
				record, err := enqueuer.Enqueue(ctx, options)
				return flowRecordOutput(record), err
			})
		},
	}
	flags.add(command, false)
	return command
}

type flowJobClaimer interface {
	ClaimJobs(context.Context, ferricstore.ClaimDueOptions) ([]ferricstore.ClaimedItem, error)
}

func newQueueClaimCommand(dependencies dependencies) *cobra.Command {
	var flags flowClaimFlags
	command := &cobra.Command{
		Use:     "claim <type>",
		Aliases: []string{"receive"},
		Short:   "Claim due jobs for a worker",
		Long: "Claim due work and create time-bounded leases. Save each returned lease_token and " +
			"fencing_token; mutations reject stale claims.",
		Example: "  ferric queue claim email --worker mailer-1\n" +
			"  ferric queue receive email --worker mailer-1 --limit 10 --lease 1m --wait 5s",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			options, err := flags.options(command, args[0])
			if err != nil {
				return err
			}
			if err := validateBlockingWait(dependencies, flags.block, "--wait"); err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "claim jobs", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				claimer, err := requireClientCapability[flowJobClaimer](client, "FerricFlow job claims")
				if err != nil {
					return nil, err
				}
				claims, err := claimer.ClaimJobs(ctx, options)
				return flowClaimsOutput(claims), err
			})
		},
	}
	flags.add(command, false)
	return command
}

type flowGetter interface {
	Get(context.Context, string, string, []string) (*ferricstore.FlowRecord, error)
}

func newFlowDescribeCommand(dependencies dependencies, service string) *cobra.Command {
	var partition string
	var values []string
	command := &cobra.Command{
		Use:     "describe <id>",
		Aliases: []string{"get", "inspect"},
		Short:   "Describe one " + service + " execution",
		Example: "  ferric " + service + " describe job-42\n" +
			"  ferric " + service + " get job-42 --partition tenant-a",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "describe "+service, func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				getter, err := requireClientCapability[flowGetter](client, "FerricFlow reads")
				if err != nil {
					return nil, err
				}
				record, err := getter.Get(ctx, args[0], partition, values)
				return flowRecordOutput(record), err
			})
		},
	}
	command.Flags().StringVar(&partition, "partition", "", "partition key")
	command.Flags().StringSliceVar(&values, "value", nil, "named value to include; repeatable")
	return command
}

type flowLister interface {
	List(context.Context, string, ferricstore.ReadOptions) ([]ferricstore.FlowRecord, error)
}

func newFlowListCommand(dependencies dependencies, service string) *cobra.Command {
	var flags flowReadFlags
	command := &cobra.Command{
		Use:   "list <type>",
		Short: "List " + service + " executions by type",
		Example: "  ferric " + service + " list email --partition tenant-a\n" +
			"  ferric " + service + " list email --partition tenant-a --state failed --limit 20 --reverse",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			options, err := flags.options(command)
			if err != nil {
				return err
			}
			if err := validateFlowListQuery(args[0], options); err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "list "+service+" executions", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				lister, err := requireClientCapability[flowLister](client, "FerricFlow list")
				if err != nil {
					return nil, err
				}
				records, err := lister.List(ctx, args[0], options)
				return flowRecordsOutput(records), err
			})
		},
	}
	flags.addQuery(command, fullFlowReadFlagSet)
	return command
}

func validateFlowListQuery(flowType string, options ferricstore.ReadOptions) error {
	if flowType == "any" && len(options.Attributes) == 0 {
		return errors.New("list requires a concrete type or an attribute predicate")
	}
	terminalOnly := options.TerminalOnly != nil && *options.TerminalOnly
	if !terminalOnly && options.State == "any" && len(options.Attributes) == 0 {
		return errors.New("list with state any requires an attribute predicate")
	}
	if terminalOnly {
		return validateTerminalQueryState(options.State)
	}
	return nil
}

func validateTerminalQueryState(state string) error {
	switch state {
	case "", "any", "completed", "failed", "cancelled":
		return nil
	default:
		return errors.New("terminal state must be completed, failed, cancelled, or any")
	}
}

type flowLeaseExtender interface {
	ExtendLease(context.Context, string, string, int64, int64, string) (*ferricstore.FlowRecord, error)
}

func newQueueExtendCommand(dependencies dependencies) *cobra.Command {
	var leaseFlags flowLeaseFlags
	var lease time.Duration
	command := &cobra.Command{
		Use:     "extend <id>",
		Short:   "Extend a claimed job lease",
		Long:    "Extend a live claim before it expires. Both tokens must come from the same claim response.",
		Example: "  ferric queue extend email-42 --lease-token TOKEN --fencing-token 1 --lease 30s",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := leaseFlags.validate(true, true); err != nil {
				return err
			}
			leaseMS, err := positiveMilliseconds("lease", lease)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "extend job lease", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				extender, err := requireClientCapability[flowLeaseExtender](client, "FerricFlow lease extension")
				if err != nil {
					return nil, err
				}
				record, err := extender.ExtendLease(ctx, args[0], leaseFlags.leaseToken, leaseFlags.fencingToken, leaseMS, leaseFlags.partition)
				return flowRecordOutput(record), err
			})
		},
	}
	leaseFlags.add(command, true, true)
	command.Flags().DurationVar(&lease, "lease", 30*time.Second, "new lease duration")
	return command
}

type flowCompleter interface {
	Complete(context.Context, ferricstore.CompleteOptions) (*ferricstore.FlowRecord, error)
}

func newFlowCompleteCommand(dependencies dependencies, service string) *cobra.Command {
	var leaseFlags flowLeaseFlags
	var input valueInput
	var ttl time.Duration
	command := &cobra.Command{
		Use:     "complete <id> [result]",
		Aliases: []string{"ack"},
		Short:   "Complete a claimed " + service + " execution",
		Long:    "Complete a claim using the lease and fencing tokens returned by claim. Alias: ack.",
		Example: "  ferric " + service + " complete job-42 --lease-token TOKEN --fencing-token 1\n" +
			"  ferric " + service + " ack job-42 '{\"sent\":true}' --json --lease-token TOKEN --fencing-token 1",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(command *cobra.Command, args []string) error {
			if err := leaseFlags.validate(true, true); err != nil {
				return err
			}
			var positional *string
			if len(args) == 2 {
				positional = &args[1]
			}
			result, present, err := input.read(command, positional)
			if err != nil {
				return err
			}
			options := ferricstore.CompleteOptions{
				ID:           args[0],
				LeaseToken:   leaseFlags.leaseToken,
				FencingToken: leaseFlags.fencingToken,
				PartitionKey: leaseFlags.partition,
				ReturnRecord: true,
			}
			if present {
				options.Result = result
			}
			if command.Flags().Changed("retention") {
				ttlMS, err := positiveMilliseconds("retention", ttl)
				if err != nil {
					return err
				}
				options.TTLMS = &ttlMS
			}
			return runNetworkCommand(command, dependencies, "complete "+service, func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				completer, err := requireClientCapability[flowCompleter](client, "FerricFlow completion")
				if err != nil {
					return nil, err
				}
				record, err := completer.Complete(ctx, options)
				return flowRecordOutput(record), err
			})
		},
	}
	leaseFlags.add(command, true, true)
	input.addFlags(command, "result")
	command.Flags().DurationVar(&ttl, "retention", 0, "terminal record retention time")
	return command
}

type flowRetrier interface {
	Retry(context.Context, ferricstore.RetryOptions) (*ferricstore.FlowRecord, error)
}

func newFlowRetryCommand(dependencies dependencies, service string) *cobra.Command {
	var leaseFlags flowLeaseFlags
	var input valueInput
	var runAt string
	var delay time.Duration
	command := &cobra.Command{
		Use:     "retry <id> [error]",
		Aliases: []string{"nack"},
		Short:   "Release a claim for retry",
		Long:    "Record an error and make the execution eligible again, immediately or at a later time. Alias: nack.",
		Example: "  ferric " + service + " retry job-42 timeout --lease-token TOKEN --fencing-token 1 --delay 30s",
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(command *cobra.Command, args []string) error {
			if err := leaseFlags.validate(true, true); err != nil {
				return err
			}
			var positional *string
			if len(args) == 2 {
				positional = &args[1]
			}
			failure, present, err := input.read(command, positional)
			if err != nil {
				return err
			}
			runAtMS, err := parseRunAt(runAt, delay, time.Now())
			if err != nil {
				return err
			}
			options := ferricstore.RetryOptions{
				ID:           args[0],
				LeaseToken:   leaseFlags.leaseToken,
				FencingToken: leaseFlags.fencingToken,
				PartitionKey: leaseFlags.partition,
				RunAtMS:      runAtMS,
				ReturnRecord: true,
			}
			if present {
				options.Error = failure
			}
			return runNetworkCommand(command, dependencies, "retry "+service, func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				retrier, err := requireClientCapability[flowRetrier](client, "FerricFlow retry")
				if err != nil {
					return nil, err
				}
				record, err := retrier.Retry(ctx, options)
				return flowRecordOutput(record), err
			})
		},
	}
	leaseFlags.add(command, true, true)
	input.addFlags(command, "error")
	command.Flags().StringVar(&runAt, "run-at", "", "RFC3339 time or Unix milliseconds for the next attempt")
	command.Flags().DurationVar(&delay, "delay", 0, "delay the next attempt by this duration")
	return command
}

type flowFailer interface {
	Fail(context.Context, ferricstore.FailOptions) (*ferricstore.FlowRecord, error)
}

func newFlowFailCommand(dependencies dependencies, service string) *cobra.Command {
	var leaseFlags flowLeaseFlags
	var input valueInput
	var ttl time.Duration
	command := &cobra.Command{
		Use:     "fail <id> [error]",
		Short:   "Move a claimed " + service + " execution to a terminal failure",
		Example: "  ferric " + service + " fail job-42 exhausted --lease-token TOKEN --fencing-token 1",
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(command *cobra.Command, args []string) error {
			if err := leaseFlags.validate(true, true); err != nil {
				return err
			}
			var positional *string
			if len(args) == 2 {
				positional = &args[1]
			}
			failure, present, err := input.read(command, positional)
			if err != nil {
				return err
			}
			options := ferricstore.FailOptions{
				ID:           args[0],
				LeaseToken:   leaseFlags.leaseToken,
				FencingToken: leaseFlags.fencingToken,
				PartitionKey: leaseFlags.partition,
				ReturnRecord: true,
			}
			if present {
				options.Error = failure
			}
			if command.Flags().Changed("retention") {
				ttlMS, err := positiveMilliseconds("retention", ttl)
				if err != nil {
					return err
				}
				options.TTLMS = &ttlMS
			}
			return runNetworkCommand(command, dependencies, "fail "+service, func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				failer, err := requireClientCapability[flowFailer](client, "FerricFlow failure")
				if err != nil {
					return nil, err
				}
				record, err := failer.Fail(ctx, options)
				return flowRecordOutput(record), err
			})
		},
	}
	leaseFlags.add(command, true, true)
	input.addFlags(command, "error")
	command.Flags().DurationVar(&ttl, "retention", 0, "terminal record retention time")
	return command
}

type flowCanceller interface {
	Cancel(context.Context, ferricstore.CancelOptions) (*ferricstore.FlowRecord, error)
}

func newFlowCancelCommand(dependencies dependencies, service string) *cobra.Command {
	var leaseFlags flowLeaseFlags
	var input valueInput
	var ttl time.Duration
	command := &cobra.Command{
		Use:     "cancel <id> [reason]",
		Short:   "Cancel a " + service + " execution",
		Long:    "Cancel an execution using its latest fencing token. Supply the lease token when cancelling claimed work.",
		Example: "  ferric " + service + " cancel job-42 obsolete --fencing-token 1",
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(command *cobra.Command, args []string) error {
			if err := leaseFlags.validate(false, false); err != nil {
				return err
			}
			var positional *string
			if len(args) == 2 {
				positional = &args[1]
			}
			reason, present, err := input.read(command, positional)
			if err != nil {
				return err
			}
			options := ferricstore.CancelOptions{
				ID:           args[0],
				LeaseToken:   leaseFlags.leaseToken,
				FencingToken: leaseFlags.fencingToken,
				PartitionKey: leaseFlags.partition,
				ReturnRecord: true,
			}
			if present {
				options.Reason = reason
			}
			if command.Flags().Changed("retention") {
				ttlMS, err := positiveMilliseconds("retention", ttl)
				if err != nil {
					return err
				}
				options.TTLMS = &ttlMS
			}
			return runNetworkCommand(command, dependencies, "cancel "+service, func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				canceller, err := requireClientCapability[flowCanceller](client, "FerricFlow cancellation")
				if err != nil {
					return nil, err
				}
				record, err := canceller.Cancel(ctx, options)
				return flowRecordOutput(record), err
			})
		},
	}
	leaseFlags.add(command, false, false)
	input.addFlags(command, "reason")
	command.Flags().DurationVar(&ttl, "retention", 0, "terminal record retention time")
	return command
}

type flowHistorian interface {
	History(context.Context, ferricstore.HistoryOptions) ([]any, error)
}

func newFlowHistoryCommand(dependencies dependencies, service string) *cobra.Command {
	var partition string
	var limit int
	var reverse bool
	var event string
	var worker string
	var includeCold bool
	var values bool
	command := &cobra.Command{
		Use:   "history <id>",
		Short: "Show the event history of a " + service + " execution",
		Example: "  ferric " + service + " history job-42\n" +
			"  ferric " + service + " history job-42 --reverse --limit 20",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if limit <= 0 {
				return errors.New("limit must be greater than zero")
			}
			options := ferricstore.HistoryOptions{
				ID:           args[0],
				PartitionKey: partition,
				Count:        limit,
				Event:        event,
				Worker:       worker,
			}
			if command.Flags().Changed("reverse") {
				options.Rev = ferricstore.Bool(reverse)
			}
			if command.Flags().Changed("include-cold") {
				options.IncludeCold = ferricstore.Bool(includeCold)
			}
			if command.Flags().Changed("values") {
				options.Values = ferricstore.Bool(values)
			}
			return runNetworkCommand(command, dependencies, service+" history", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				historian, err := requireClientCapability[flowHistorian](client, "FerricFlow history")
				if err != nil {
					return nil, err
				}
				return historian.History(ctx, options)
			})
		},
	}
	command.Flags().StringVar(&partition, "partition", "", "partition key")
	command.Flags().IntVar(&limit, "limit", 100, "maximum events to return")
	command.Flags().BoolVar(&reverse, "reverse", false, "return newest events first")
	command.Flags().StringVar(&event, "event", "", "filter by event type")
	command.Flags().StringVar(&worker, "worker", "", "filter by worker")
	command.Flags().BoolVar(&includeCold, "include-cold", false, "include retained cold history")
	command.Flags().BoolVar(&values, "values", false, "include named values")
	return command
}

type flowStatsReader interface {
	Stats(context.Context, string, ferricstore.ReadOptions) (map[string]any, error)
}

func newFlowStatsCommand(dependencies dependencies, service string) *cobra.Command {
	var flags flowReadFlags
	command := &cobra.Command{
		Use:   "stats <type>",
		Short: "Summarize " + service + " executions by state",
		Example: "  ferric " + service + " stats email\n" +
			"  ferric " + service + " stats email --partition tenant-a",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			options, err := flags.options(command)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, service+" statistics", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[flowStatsReader](client, "FerricFlow statistics")
				if err != nil {
					return nil, err
				}
				return reader.Stats(ctx, args[0], options)
			})
		},
	}
	flags.addAdmin(command, true)
	return command
}

func newFlowPolicyCommand(dependencies dependencies, service string) *cobra.Command {
	command := &cobra.Command{
		Use:   "policy",
		Short: "Inspect or change " + service + " execution policy",
		Example: "  ferric " + service + " policy get email\n" +
			"  ferric " + service + " policy set email --max-retries 5",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newFlowPolicyGetCommand(dependencies, service),
		newFlowPolicySetCommand(dependencies, service),
	)
	return command
}

type flowPolicyGetter interface {
	PolicyGet(context.Context, string, string) (ferricstore.PolicySnapshot, error)
}

func newFlowPolicyGetCommand(dependencies dependencies, service string) *cobra.Command {
	var state string
	command := &cobra.Command{
		Use:   "get <type>",
		Short: "Get the effective policy for a type",
		Example: "  ferric " + service + " policy get email\n" +
			"  ferric " + service + " policy get email --state queued",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "get "+service+" policy", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				getter, err := requireClientCapability[flowPolicyGetter](client, "FerricFlow policy reads")
				if err != nil {
					return nil, err
				}
				result, err := getter.PolicyGet(ctx, args[0], state)
				return outputcontract.Policy(result), err
			})
		},
	}
	command.Flags().StringVar(&state, "state", "", "state-specific policy")
	_ = command.RegisterFlagCompletionFunc("state", completeCommonFlowState)
	return command
}

type flowPolicySetter interface {
	SetPolicy(context.Context, string, ferricstore.PolicyOptions) (ferricstore.PolicySnapshot, error)
}

type flowPolicySetFlags struct {
	maxActive              string
	maxRetries             int
	backoff                string
	baseDelay              time.Duration
	maxDelay               time.Duration
	jitterPercent          int
	exhaustedTo            string
	replace                bool
	expectedGeneration     int64
	indexedAttributes      []string
	clearIndexedAttributes bool
	indexedStateMeta       string
	stateModes             []string
}

func newFlowPolicySetCommand(dependencies dependencies, service string) *cobra.Command {
	var flags flowPolicySetFlags
	var yes bool
	command := &cobra.Command{
		Use:   "set <type>",
		Short: "Set retry, deadline, index, or state-mode policy",
		Long: "Patch policy fields for a type. This command requires --yes. State modes use state=FIFO or state=PARALLEL. " +
			"Omitted fields retain their current values.",
		Example: "  ferric " + service + " policy set email --max-retries 5 --backoff exponential --base-delay 1s --max-delay 1m --yes\n" +
			"  ferric " + service + " policy set email --state-mode queued=FIFO --yes",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := requireProtocolConfirmation(yes, service+" policy update", []any{"FLOW.POLICY.SET"}); err != nil {
				return err
			}
			options, err := flags.options(command)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "set "+service+" policy", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				setter, err := requireClientCapability[flowPolicySetter](client, "FerricFlow policy changes")
				if err != nil {
					return nil, err
				}
				result, err := setter.SetPolicy(ctx, args[0], options)
				return outputcontract.Policy(result), err
			})
		},
	}
	command.Flags().StringVar(&flags.maxActive, "max-active", "", "maximum active time, such as 5m or infinity")
	command.Flags().IntVar(&flags.maxRetries, "max-retries", 0, "maximum automatic retries")
	command.Flags().StringVar(&flags.backoff, "backoff", "", "retry backoff algorithm")
	command.Flags().DurationVar(&flags.baseDelay, "base-delay", 0, "base retry delay")
	command.Flags().DurationVar(&flags.maxDelay, "max-delay", 0, "maximum retry delay")
	command.Flags().IntVar(&flags.jitterPercent, "jitter-percent", 0, "retry jitter percentage")
	command.Flags().StringVar(&flags.exhaustedTo, "exhausted-to", "", "state entered after retries are exhausted")
	command.Flags().BoolVar(&flags.replace, "replace", false, "replace policy instead of patching it")
	command.Flags().Int64Var(&flags.expectedGeneration, "expected-generation", 0, "only update the expected policy generation")
	command.Flags().StringSliceVar(&flags.indexedAttributes, "indexed-attribute", nil, "searchable attribute; repeatable")
	command.Flags().BoolVar(&flags.clearIndexedAttributes, "clear-indexed-attributes", false, "remove all indexed attributes")
	command.Flags().StringVar(&flags.indexedStateMeta, "indexed-state-meta", "", "state metadata field to index; use an empty value to clear")
	command.Flags().StringArrayVar(&flags.stateModes, "state-mode", nil, "state claim mode as state=FIFO or state=PARALLEL; repeatable")
	command.Flags().BoolVar(&yes, "yes", false, "confirm the policy update")
	return command
}

func (flags flowPolicySetFlags) options(command *cobra.Command) (ferricstore.PolicyOptions, error) {
	var options ferricstore.PolicyOptions
	changed := false
	if command.Flags().Changed("max-active") {
		value, err := parseMaxActive(flags.maxActive)
		if err != nil {
			return options, err
		}
		options.MaxActiveMS = value
		changed = true
	}
	retryChanged := false
	retry := &ferricstore.RetryPolicy{}
	if command.Flags().Changed("max-retries") {
		if flags.maxRetries < 0 {
			return options, errors.New("max-retries must not be negative")
		}
		retry.MaxRetries = flags.maxRetries
		retry.MaxRetriesSet = true
		retryChanged = true
	}
	if command.Flags().Changed("backoff") {
		retry.Backoff = flags.backoff
		retryChanged = true
	}
	if command.Flags().Changed("base-delay") {
		value, err := positiveMilliseconds("base-delay", flags.baseDelay)
		if err != nil {
			return options, err
		}
		retry.BaseMS = value
		retry.BaseMSSet = true
		retryChanged = true
	}
	if command.Flags().Changed("max-delay") {
		value, err := positiveMilliseconds("max-delay", flags.maxDelay)
		if err != nil {
			return options, err
		}
		retry.MaxMS = value
		retry.MaxMSSet = true
		retryChanged = true
	}
	if command.Flags().Changed("jitter-percent") {
		if flags.jitterPercent < 0 || flags.jitterPercent > 100 {
			return options, errors.New("jitter-percent must be between 0 and 100")
		}
		retry.JitterPct = flags.jitterPercent
		retry.JitterPctSet = true
		retryChanged = true
	}
	if command.Flags().Changed("exhausted-to") {
		retry.ExhaustedTo = flags.exhaustedTo
		retryChanged = true
	}
	if retryChanged {
		options.Retry = retry
		changed = true
	}
	if command.Flags().Changed("replace") {
		options.Replace = ferricstore.Bool(flags.replace)
		changed = true
	}
	if command.Flags().Changed("expected-generation") {
		if flags.expectedGeneration < 0 {
			return options, errors.New("expected-generation must not be negative")
		}
		options.ExpectedGeneration = ferricstore.Int64(flags.expectedGeneration)
		changed = true
	}
	if flags.clearIndexedAttributes && len(flags.indexedAttributes) != 0 {
		return options, errors.New("use --indexed-attribute or --clear-indexed-attributes, not both")
	}
	if flags.clearIndexedAttributes {
		options.IndexedAttributes = []string{}
		changed = true
	} else if command.Flags().Changed("indexed-attribute") {
		options.IndexedAttributes = flags.indexedAttributes
		changed = true
	}
	if command.Flags().Changed("indexed-state-meta") {
		options.IndexedStateMeta = flags.indexedStateMeta
		options.IndexedStateMetaSet = true
		changed = true
	}
	if len(flags.stateModes) != 0 {
		policies, err := parseStateModes(flags.stateModes)
		if err != nil {
			return options, err
		}
		options.StatePolicies = policies
		changed = true
	}
	if !changed {
		return options, errors.New("at least one policy option is required")
	}
	return options, nil
}

func parseStateModes(values []string) (map[string]ferricstore.FlowStatePolicy, error) {
	result := make(map[string]ferricstore.FlowStatePolicy, len(values))
	for _, item := range values {
		state, rawMode, found := strings.Cut(item, "=")
		state = strings.TrimSpace(state)
		mode := ferricstore.FlowStateMode(strings.ToUpper(strings.TrimSpace(rawMode)))
		if !found || state == "" || (mode != ferricstore.FlowStateModeFIFO && mode != ferricstore.FlowStateModeParallel) {
			return nil, errors.New("state-mode must use state=FIFO or state=PARALLEL")
		}
		if _, duplicate := result[state]; duplicate {
			return nil, errors.New("duplicate state-mode for " + state)
		}
		result[state] = ferricstore.FlowStatePolicy{Mode: mode}
	}
	return result, nil
}
