package cli

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	ferricstore "github.com/ferricstore/ferricstore-go"
	"github.com/spf13/cobra"
)

func newWorkflowCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "workflow",
		Short: "Start, inspect, and control FerricFlow workflows",
		Long: "Manage stateful FerricFlow executions using familiar workflow operations. " +
			"Queue jobs are Flow executions and can also be inspected here. " +
			"Schedule definitions and governance controls are nested under this service.",
		Example: "  ferric workflow start order order-42 '{\"total\":125}' --json\n" +
			"  ferric workflow describe order-42\n" +
			"  ferric workflow history order-42\n" +
			"  ferric workflow signal order-42 payment-received\n" +
			"  ferric workflow schedule list\n" +
			"  ferric workflow governance overview",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newWorkflowStartCommand(dependencies),
		newFlowDescribeCommand(dependencies, "workflow"),
		newFlowListCommand(dependencies, "workflow"),
		newWorkflowSearchCommand(dependencies),
		newWorkflowClaimCommand(dependencies),
		newWorkflowSignalCommand(dependencies),
		newWorkflowTransitionCommand(dependencies),
		newFlowCompleteCommand(dependencies, "workflow"),
		newFlowRetryCommand(dependencies, "workflow"),
		newFlowFailCommand(dependencies, "workflow"),
		newFlowCancelCommand(dependencies, "workflow"),
		newWorkflowRewindCommand(dependencies),
		newFlowHistoryCommand(dependencies, "workflow"),
		newWorkflowChildrenCommand(dependencies),
		newWorkflowByRootCommand(dependencies),
		newWorkflowByCorrelationCommand(dependencies),
		newWorkflowTerminalsCommand(dependencies),
		newWorkflowFailuresCommand(dependencies),
		newWorkflowStuckCommand(dependencies),
		newWorkflowInfoCommand(dependencies),
		newWorkflowValuesCommand(dependencies),
		newFlowStatsCommand(dependencies, "workflow"),
		newFlowPolicyCommand(dependencies, "workflow"),
		newScheduleCommand(dependencies),
		newGovernanceCommand(dependencies),
	)
	return command
}

type flowCreator interface {
	Create(context.Context, ferricstore.CreateOptions) (*ferricstore.FlowRecord, error)
}

func newWorkflowStartCommand(dependencies dependencies) *cobra.Command {
	var flags flowCreateFlags
	command := &cobra.Command{
		Use:   "start <type> <id> [payload]",
		Short: "Start a workflow execution",
		Long:  "Create a stateful execution with a stable ID, initial state, payload, and optional searchable metadata.",
		Example: "  ferric workflow start order order-42 '{\"total\":125}' --json\n" +
			"  ferric workflow start child child-42 --parent order-42 --root order-42 --partition tenant-a",
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
			return runNetworkCommand(command, dependencies, "start workflow", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				creator, err := requireClientCapability[flowCreator](client, "FerricFlow creation")
				if err != nil {
					return nil, err
				}
				record, err := creator.Create(ctx, options)
				return flowRecordOutput(record), err
			})
		},
	}
	flags.add(command, true)
	return command
}

type flowClaimer interface {
	ClaimDue(context.Context, ferricstore.ClaimDueOptions) ([]ferricstore.FlowRecord, error)
}

func newWorkflowClaimCommand(dependencies dependencies) *cobra.Command {
	var flags flowClaimFlags
	command := &cobra.Command{
		Use:   "claim <type>",
		Short: "Claim due workflow executions for a worker",
		Long:  "Claim one or more due executions and return their complete Flow records, lease tokens, and fencing tokens.",
		Example: "  ferric workflow claim order --state queued --worker order-worker-1\n" +
			"  ferric workflow claim order --state payment-pending --worker payment-1 --limit 10 --lease 1m",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			options, err := flags.options(command, args[0])
			if err != nil {
				return err
			}
			options.JobOnly = false
			return runNetworkCommand(command, dependencies, "claim workflows", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				claimer, err := requireClientCapability[flowClaimer](client, "FerricFlow claims")
				if err != nil {
					return nil, err
				}
				records, err := claimer.ClaimDue(ctx, options)
				return flowRecordsOutput(records), err
			})
		},
	}
	flags.add(command)
	return command
}

type flowSearcher interface {
	Search(context.Context, ferricstore.SearchOptions) ([]ferricstore.FlowRecord, error)
}

func newWorkflowSearchCommand(dependencies dependencies) *cobra.Command {
	var flowType string
	var state string
	var partition string
	var limit int
	var reverse bool
	var terminalOnly bool
	var includeCold bool
	var consistent bool
	var attributes []string
	command := &cobra.Command{
		Use:   "search",
		Short: "Search workflow executions",
		Long:  "Search bounded Flow indexes by type, state, partition, time-independent attributes, or terminal status.",
		Example: "  ferric workflow search --type order --state failed --limit 20\n" +
			"  ferric workflow search --type order --partition tenant-a --attribute region=eu --reverse",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if limit <= 0 {
				return errors.New("limit must be greater than zero")
			}
			filters, err := parseAssignments(attributes, "attribute")
			if err != nil {
				return err
			}
			options := ferricstore.SearchOptions{
				Type:         flowType,
				State:        state,
				PartitionKey: partition,
				Count:        ferricstore.Int(limit),
				Attributes:   filters,
			}
			if command.Flags().Changed("reverse") {
				options.Rev = ferricstore.Bool(reverse)
			}
			if command.Flags().Changed("terminal-only") {
				options.TerminalOnly = ferricstore.Bool(terminalOnly)
			}
			if command.Flags().Changed("include-cold") {
				options.IncludeCold = ferricstore.Bool(includeCold)
			}
			if command.Flags().Changed("consistent") {
				options.ConsistentProjection = ferricstore.Bool(consistent)
			}
			return runNetworkCommand(command, dependencies, "search workflows", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				searcher, err := requireClientCapability[flowSearcher](client, "FerricFlow search")
				if err != nil {
					return nil, err
				}
				records, err := searcher.Search(ctx, options)
				return flowRecordsOutput(records), err
			})
		},
	}
	command.Flags().StringVar(&flowType, "type", "", "filter by workflow type")
	command.Flags().StringVar(&state, "state", "", "filter by state")
	command.Flags().StringVar(&partition, "partition", "", "partition key")
	command.Flags().IntVar(&limit, "limit", 100, "maximum records to return")
	command.Flags().BoolVar(&reverse, "reverse", false, "return newest records first")
	command.Flags().BoolVar(&terminalOnly, "terminal-only", false, "return only terminal records")
	command.Flags().BoolVar(&includeCold, "include-cold", false, "include retained cold records")
	command.Flags().BoolVar(&consistent, "consistent", false, "wait for a consistent cold projection")
	command.Flags().StringArrayVar(&attributes, "attribute", nil, "filter attribute as name=value; repeatable")
	_ = command.RegisterFlagCompletionFunc("state", completeCommonFlowState)
	return command
}

type flowSignaler interface {
	Signal(context.Context, ferricstore.SignalOptions) (any, error)
}

func newWorkflowSignalCommand(dependencies dependencies) *cobra.Command {
	var partition string
	var idempotencyKey string
	var ifStates []string
	var transitionTo string
	var runAt string
	var delay time.Duration
	var values []string
	var attributes []string
	var dataName string
	var input valueInput
	command := &cobra.Command{
		Use:   "signal <id> <signal> [data]",
		Short: "Send a signal to a workflow execution",
		Long:  "Send an idempotent event with optional named data and an atomic state transition.",
		Example: "  ferric workflow signal order-42 payment-received\n" +
			"  ferric workflow signal order-42 approved '{\"by\":\"ada\"}' --json --idempotency-key approval-7 --if-state review --transition-to approved",
		Args: cobra.RangeArgs(2, 3),
		RunE: func(command *cobra.Command, args []string) error {
			var positional *string
			if len(args) == 3 {
				positional = &args[2]
			}
			data, present, err := input.read(command, positional)
			if err != nil {
				return err
			}
			namedValues, err := parseAssignments(values, "value")
			if err != nil {
				return err
			}
			if present {
				if strings.TrimSpace(dataName) == "" {
					return errors.New("data-name must not be empty")
				}
				if namedValues == nil {
					namedValues = make(map[string]any)
				}
				if _, duplicate := namedValues[dataName]; duplicate {
					return errors.New("signal data duplicates --value " + dataName)
				}
				namedValues[dataName] = data
			}
			attributeMerge, err := parseAssignments(attributes, "attribute")
			if err != nil {
				return err
			}
			runAtMS, err := parseRunAt(runAt, delay, time.Now())
			if err != nil {
				return err
			}
			options := ferricstore.SignalOptions{
				ID:             args[0],
				Signal:         args[1],
				PartitionKey:   partition,
				IdempotencyKey: idempotencyKey,
				IfStates:       ifStates,
				TransitionTo:   transitionTo,
				RunAtMS:        runAtMS,
				NamedValues: ferricstore.NamedValues{
					Values:          namedValues,
					AttributesMerge: attributeMerge,
				},
			}
			return runNetworkCommand(command, dependencies, "signal workflow", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				signaler, err := requireClientCapability[flowSignaler](client, "FerricFlow signals")
				if err != nil {
					return nil, err
				}
				return signaler.Signal(ctx, options)
			})
		},
	}
	command.Flags().StringVar(&partition, "partition", "", "partition key")
	command.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "deduplication key for this signal")
	command.Flags().StringSliceVar(&ifStates, "if-state", nil, "only apply while in this state; repeatable")
	command.Flags().StringVar(&transitionTo, "transition-to", "", "atomically transition to this state")
	command.Flags().StringVar(&runAt, "run-at", "", "RFC3339 time or Unix milliseconds for the transition")
	command.Flags().DurationVar(&delay, "delay", 0, "delay the transition by this duration")
	command.Flags().StringArrayVar(&values, "value", nil, "named signal value as name=value; repeatable")
	command.Flags().StringArrayVar(&attributes, "attribute", nil, "attribute to merge as name=value; repeatable")
	command.Flags().StringVar(&dataName, "data-name", "data", "name used for positional signal data")
	input.addFlags(command, "signal data")
	_ = command.RegisterFlagCompletionFunc("if-state", completeCommonFlowState)
	_ = command.RegisterFlagCompletionFunc("transition-to", completeCommonFlowState)
	return command
}

type flowTransitioner interface {
	Transition(context.Context, ferricstore.TransitionOptions) (*ferricstore.FlowRecord, error)
}

func newWorkflowTransitionCommand(dependencies dependencies) *cobra.Command {
	var leaseFlags flowLeaseFlags
	var runAt string
	var delay time.Duration
	var priority int64
	var attributes []string
	var deleteAttributes []string
	var input valueInput
	command := &cobra.Command{
		Use:   "transition <id> <from-state> <to-state> [payload]",
		Short: "Atomically transition a workflow state",
		Long:  "Transition only from the expected state and fencing generation. A live lease token is required.",
		Example: "  ferric workflow transition order-42 queued charged --lease-token TOKEN --fencing-token 1\n" +
			"  ferric workflow transition order-42 review approved --lease-token TOKEN --fencing-token 2 --attribute approver=ada",
		Args: cobra.RangeArgs(3, 4),
		RunE: func(command *cobra.Command, args []string) error {
			if err := leaseFlags.validate(true, true); err != nil {
				return err
			}
			var positional *string
			if len(args) == 4 {
				positional = &args[3]
			}
			payload, present, err := input.read(command, positional)
			if err != nil {
				return err
			}
			runAtMS, err := parseRunAt(runAt, delay, time.Now())
			if err != nil {
				return err
			}
			attributeMerge, err := parseAssignments(attributes, "attribute")
			if err != nil {
				return err
			}
			options := ferricstore.TransitionOptions{
				ID:           args[0],
				FromState:    args[1],
				ToState:      args[2],
				LeaseToken:   leaseFlags.leaseToken,
				FencingToken: leaseFlags.fencingToken,
				PartitionKey: leaseFlags.partition,
				RunAtMS:      runAtMS,
				ReturnRecord: true,
				NamedValues: ferricstore.NamedValues{
					AttributesMerge:  attributeMerge,
					AttributesDelete: deleteAttributes,
				},
			}
			if present {
				options.Payload = payload
			}
			if command.Flags().Changed("priority") {
				options.Priority = ferricstore.Int64(priority)
			}
			return runNetworkCommand(command, dependencies, "transition workflow", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				transitioner, err := requireClientCapability[flowTransitioner](client, "FerricFlow transitions")
				if err != nil {
					return nil, err
				}
				record, err := transitioner.Transition(ctx, options)
				return flowRecordOutput(record), err
			})
		},
	}
	leaseFlags.add(command, true, true)
	command.Flags().StringVar(&runAt, "run-at", "", "RFC3339 time or Unix milliseconds at which the new state becomes due")
	command.Flags().DurationVar(&delay, "delay", 0, "delay the new state by this duration")
	command.Flags().Int64Var(&priority, "priority", 0, "new scheduling priority")
	command.Flags().StringArrayVar(&attributes, "attribute", nil, "attribute to merge as name=value; repeatable")
	command.Flags().StringSliceVar(&deleteAttributes, "delete-attribute", nil, "attribute to remove; repeatable")
	input.addFlags(command, "payload")
	return command
}

type flowRewinder interface {
	Rewind(context.Context, ferricstore.RewindOptions) (*ferricstore.FlowRecord, error)
}

func newWorkflowRewindCommand(dependencies dependencies) *cobra.Command {
	var partition string
	var toEvent string
	var expectState string
	var runAt string
	var delay time.Duration
	command := &cobra.Command{
		Use:     "rewind <id>",
		Short:   "Rewind a workflow to a prior event",
		Long:    "Create a new current state from a historical event, optionally guarded by the current state.",
		Example: "  ferric workflow rewind order-42 --to-event event-17 --expect-state failed",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(toEvent) == "" {
				return errors.New("to-event is required; use --to-event <event-id>")
			}
			runAtMS, err := parseRunAt(runAt, delay, time.Now())
			if err != nil {
				return err
			}
			options := ferricstore.RewindOptions{
				ID:           args[0],
				ToEvent:      toEvent,
				PartitionKey: partition,
				ExpectState:  expectState,
				RunAtMS:      runAtMS,
				ReturnRecord: true,
			}
			return runNetworkCommand(command, dependencies, "rewind workflow", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				rewinder, err := requireClientCapability[flowRewinder](client, "FerricFlow rewind")
				if err != nil {
					return nil, err
				}
				record, err := rewinder.Rewind(ctx, options)
				return flowRecordOutput(record), err
			})
		},
	}
	command.Flags().StringVar(&partition, "partition", "", "partition key")
	command.Flags().StringVar(&toEvent, "to-event", "", "historical event ID (required)")
	command.Flags().StringVar(&expectState, "expect-state", "", "only rewind from this current state")
	command.Flags().StringVar(&runAt, "run-at", "", "RFC3339 time or Unix milliseconds at which work becomes due")
	command.Flags().DurationVar(&delay, "delay", 0, "make rewound work due after this duration")
	_ = command.RegisterFlagCompletionFunc("expect-state", completeCommonFlowState)
	return command
}

type flowByParentReader interface {
	ByParent(context.Context, string, ferricstore.ReadOptions) ([]ferricstore.FlowRecord, error)
}

func newWorkflowChildrenCommand(dependencies dependencies) *cobra.Command {
	return newWorkflowIndexCommand(dependencies, workflowIndexSpec{
		use: "children <id>", short: "List child workflows", example: "  ferric workflow children order-42",
		operation: "list child workflows",
		read: func(ctx context.Context, client connection.Client, key string, options ferricstore.ReadOptions) ([]ferricstore.FlowRecord, error) {
			reader, err := requireClientCapability[flowByParentReader](client, "FerricFlow parent lineage")
			if err != nil {
				return nil, err
			}
			return reader.ByParent(ctx, key, options)
		},
	})
}

type flowByRootReader interface {
	ByRoot(context.Context, string, ferricstore.ReadOptions) ([]ferricstore.FlowRecord, error)
}

func newWorkflowByRootCommand(dependencies dependencies) *cobra.Command {
	return newWorkflowIndexCommand(dependencies, workflowIndexSpec{
		use: "by-root <id>", short: "List workflows in a root execution tree", example: "  ferric workflow by-root order-42",
		operation: "list workflows by root",
		read: func(ctx context.Context, client connection.Client, key string, options ferricstore.ReadOptions) ([]ferricstore.FlowRecord, error) {
			reader, err := requireClientCapability[flowByRootReader](client, "FerricFlow root lineage")
			if err != nil {
				return nil, err
			}
			return reader.ByRoot(ctx, key, options)
		},
	})
}

type flowByCorrelationReader interface {
	ByCorrelation(context.Context, string, ferricstore.ReadOptions) ([]ferricstore.FlowRecord, error)
}

func newWorkflowByCorrelationCommand(dependencies dependencies) *cobra.Command {
	return newWorkflowIndexCommand(dependencies, workflowIndexSpec{
		use: "by-correlation <id>", short: "List workflows by correlation ID", example: "  ferric workflow by-correlation checkout-42",
		operation: "list workflows by correlation",
		read: func(ctx context.Context, client connection.Client, key string, options ferricstore.ReadOptions) ([]ferricstore.FlowRecord, error) {
			reader, err := requireClientCapability[flowByCorrelationReader](client, "FerricFlow correlation reads")
			if err != nil {
				return nil, err
			}
			return reader.ByCorrelation(ctx, key, options)
		},
	})
}

type workflowIndexSpec struct {
	use       string
	short     string
	example   string
	operation string
	read      func(context.Context, connection.Client, string, ferricstore.ReadOptions) ([]ferricstore.FlowRecord, error)
}

func newWorkflowIndexCommand(dependencies dependencies, spec workflowIndexSpec) *cobra.Command {
	var flags flowReadFlags
	command := &cobra.Command{
		Use:     spec.use,
		Short:   spec.short,
		Example: spec.example,
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			options, err := flags.options(command)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, spec.operation, func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				records, err := spec.read(ctx, client, args[0], options)
				return flowRecordsOutput(records), err
			})
		},
	}
	flags.add(command, true)
	return command
}

type flowTerminalsReader interface {
	Terminals(context.Context, string, ferricstore.ReadOptions) ([]ferricstore.FlowRecord, error)
}

func newWorkflowTerminalsCommand(dependencies dependencies) *cobra.Command {
	return newWorkflowIndexCommand(dependencies, workflowIndexSpec{
		use: "terminals <type>", short: "List terminal workflow executions", example: "  ferric workflow terminals order --limit 20 --reverse",
		operation: "list terminal workflows",
		read: func(ctx context.Context, client connection.Client, key string, options ferricstore.ReadOptions) ([]ferricstore.FlowRecord, error) {
			reader, err := requireClientCapability[flowTerminalsReader](client, "FerricFlow terminal reads")
			if err != nil {
				return nil, err
			}
			return reader.Terminals(ctx, key, options)
		},
	})
}

type flowFailuresReader interface {
	Failures(context.Context, string, ferricstore.ReadOptions) ([]ferricstore.FlowRecord, error)
}

func newWorkflowFailuresCommand(dependencies dependencies) *cobra.Command {
	return newWorkflowIndexCommand(dependencies, workflowIndexSpec{
		use: "failures <type>", short: "List failed workflow executions", example: "  ferric workflow failures order --limit 20 --reverse",
		operation: "list failed workflows",
		read: func(ctx context.Context, client connection.Client, key string, options ferricstore.ReadOptions) ([]ferricstore.FlowRecord, error) {
			reader, err := requireClientCapability[flowFailuresReader](client, "FerricFlow failure reads")
			if err != nil {
				return nil, err
			}
			return reader.Failures(ctx, key, options)
		},
	})
}

type flowStuckReader interface {
	Stuck(context.Context, string, string, *int, *int64, *int64) ([]ferricstore.FlowRecord, error)
}

func newWorkflowStuckCommand(dependencies dependencies) *cobra.Command {
	var partition string
	var limit int
	var olderThan time.Duration
	command := &cobra.Command{
		Use:     "stuck <type>",
		Short:   "List workflows that have remained active too long",
		Example: "  ferric workflow stuck order --older-than 15m --limit 20",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if limit <= 0 {
				return errors.New("limit must be greater than zero")
			}
			var olderThanMS *int64
			if command.Flags().Changed("older-than") {
				value, err := positiveMilliseconds("older-than", olderThan)
				if err != nil {
					return err
				}
				olderThanMS = &value
			}
			return runNetworkCommand(command, dependencies, "list stuck workflows", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[flowStuckReader](client, "FerricFlow stuck reads")
				if err != nil {
					return nil, err
				}
				records, err := reader.Stuck(ctx, args[0], partition, ferricstore.Int(limit), olderThanMS, nil)
				return flowRecordsOutput(records), err
			})
		},
	}
	command.Flags().StringVar(&partition, "partition", "", "partition key")
	command.Flags().IntVar(&limit, "limit", 100, "maximum records to return")
	command.Flags().DurationVar(&olderThan, "older-than", 0, "minimum active duration")
	return command
}

type flowInfoReader interface {
	Info(context.Context, string, string, *bool, *bool) (map[string]any, error)
}

func newWorkflowInfoCommand(dependencies dependencies) *cobra.Command {
	var partition string
	var includeCold bool
	var consistent bool
	command := &cobra.Command{
		Use:   "info <type>",
		Short: "Show type-level workflow information",
		Example: "  ferric workflow info order\n" +
			"  ferric workflow info order --partition tenant-a --include-cold",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			var includeColdPointer *bool
			var consistentPointer *bool
			if command.Flags().Changed("include-cold") {
				includeColdPointer = ferricstore.Bool(includeCold)
			}
			if command.Flags().Changed("consistent") {
				consistentPointer = ferricstore.Bool(consistent)
			}
			return runNetworkCommand(command, dependencies, "workflow information", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[flowInfoReader](client, "FerricFlow information")
				if err != nil {
					return nil, err
				}
				return reader.Info(ctx, args[0], partition, includeColdPointer, consistentPointer)
			})
		},
	}
	command.Flags().StringVar(&partition, "partition", "", "partition key")
	command.Flags().BoolVar(&includeCold, "include-cold", false, "include retained cold records")
	command.Flags().BoolVar(&consistent, "consistent", false, "wait for a consistent cold projection")
	return command
}

func newWorkflowValuesCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "values",
		Short:   "Read content-addressed FerricFlow values",
		Example: "  ferric workflow values get value-ref-1 value-ref-2",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(newWorkflowValuesGetCommand(dependencies))
	return command
}

type flowValueReader interface {
	ValueMGet(context.Context, []string, *int64) ([]any, error)
}

func newWorkflowValuesGetCommand(dependencies dependencies) *cobra.Command {
	var maxBytes int64
	command := &cobra.Command{
		Use:   "get <reference>...",
		Short: "Get one or more values by reference",
		Example: "  ferric workflow values get value-ref-1 value-ref-2\n" +
			"  ferric workflow values get value-ref-1 --max-bytes 1048576",
		Args: cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			var limit *int64
			if command.Flags().Changed("max-bytes") {
				if maxBytes <= 0 {
					return errors.New("max-bytes must be greater than zero")
				}
				limit = &maxBytes
			}
			return runNetworkCommand(command, dependencies, "get workflow values", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[flowValueReader](client, "FerricFlow value reads")
				if err != nil {
					return nil, err
				}
				return reader.ValueMGet(ctx, args, limit)
			})
		},
	}
	command.Flags().Int64Var(&maxBytes, "max-bytes", 0, "maximum decoded bytes")
	return command
}
