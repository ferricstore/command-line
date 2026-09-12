package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	ferricstore "github.com/ferricstore/ferricstore-go"
	"github.com/spf13/cobra"
)

type workflowStartAndClaimer interface {
	StartAndClaim(context.Context, ferricstore.StartAndClaimOptions) (*ferricstore.FlowRecord, error)
}

type spawnChildInput struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Payload      any               `json:"payload,omitempty"`
	PartitionKey string            `json:"partition_key,omitempty"`
	Values       map[string]any    `json:"values,omitempty"`
	ValueRefs    map[string]string `json:"value_refs,omitempty"`
	Attributes   map[string]any    `json:"attributes,omitempty"`
	StateMeta    map[string]any    `json:"state_meta,omitempty"`
	MaxActiveMS  any               `json:"max_active_ms,omitempty"`
}

type workflowChildrenSpawner interface {
	SpawnChildren(context.Context, ferricstore.SpawnChildrenOptions) (any, error)
}

func newWorkflowSpawnChildrenCommand(dependencies dependencies) *cobra.Command {
	var (
		leaseFlags     flowLeaseFlags
		path           string
		group          string
		wait           string
		waitState      string
		success        string
		failure        string
		fromState      string
		onChildFailed  string
		onParentClosed string
		maxActive      string
		values         []string
	)
	command := &cobra.Command{
		Use:   "spawn-children <parent-id>",
		Short: "Create a child-workflow group atomically",
		Long:  "Create child workflows from a bounded JSON array and configure how the parent waits for their outcomes.",
		Example: "  ferric workflow spawn-children order-42 --file children.json --fencing-token 7 --group fulfillment --wait all\n" +
			"  ferric workflow spawn-children order-42 --file - --lease-token TOKEN --fencing-token 7 --success completed --failure failed",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(path) == "" {
				return errors.New("file is required; use --file <path> or --file -")
			}
			if err := leaseFlags.validate(false, true); err != nil {
				return err
			}
			contents, err := readBoundedQueryInput(command, path, maxValueInputBytes, "children input")
			if err != nil {
				return err
			}
			var inputs []spawnChildInput
			if err := json.Unmarshal(contents, &inputs); err != nil {
				return errors.New("parse children JSON: " + err.Error())
			}
			children := make([]ferricstore.ChildSpec, len(inputs))
			for index, input := range inputs {
				children[index] = ferricstore.ChildSpec{
					ID:           input.ID,
					Type:         input.Type,
					Payload:      input.Payload,
					PartitionKey: input.PartitionKey,
					Values:       input.Values,
					ValueRefs:    input.ValueRefs,
					Attributes:   input.Attributes,
					StateMeta:    input.StateMeta,
					MaxActiveMS:  input.MaxActiveMS,
				}
			}
			sharedValues, err := parseAssignments(values, "value")
			if err != nil {
				return err
			}
			parsedMaxActive, err := parseMaxActive(maxActive)
			if err != nil {
				return err
			}
			fencing := leaseFlags.fencingToken
			options := ferricstore.SpawnChildrenOptions{
				ID:             args[0],
				Children:       children,
				PartitionKey:   leaseFlags.partition,
				LeaseToken:     leaseFlags.leaseToken,
				FencingToken:   &fencing,
				GroupID:        group,
				Wait:           wait,
				WaitState:      waitState,
				Success:        success,
				Failure:        failure,
				FromState:      fromState,
				OnChildFailed:  onChildFailed,
				OnParentClosed: onParentClosed,
				MaxActiveMS:    parsedMaxActive,
				Values:         sharedValues,
			}
			return runNetworkCommand(command, dependencies, "spawn child workflows", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				spawner, err := requireClientCapability[workflowChildrenSpawner](client, "FerricFlow child spawning")
				if err != nil {
					return nil, err
				}
				return spawner.SpawnChildren(ctx, options)
			})
		},
	}
	leaseFlags.add(command, false, true)
	command.Flags().StringVar(&path, "file", "", "JSON array of child specifications; use - for stdin (required)")
	command.Flags().StringVar(&group, "group", "default", "child group ID")
	command.Flags().StringVar(&wait, "wait", "all", "wait policy: all, any, or none")
	command.Flags().StringVar(&waitState, "wait-state", "", "parent state while children are pending")
	command.Flags().StringVar(&success, "success", "", "parent state when the wait policy succeeds")
	command.Flags().StringVar(&failure, "failure", "", "parent state when the wait policy fails")
	command.Flags().StringVar(&fromState, "from-state", "", "only spawn while the parent is in this state")
	command.Flags().StringVar(&onChildFailed, "on-child-failed", "", "fail_parent or ignore")
	command.Flags().StringVar(&onParentClosed, "on-parent-closed", "", "cancel_children or abandon_children")
	command.Flags().StringVar(&maxActive, "max-active", "", "shared maximum active time, such as 5m or infinity")
	command.Flags().StringArrayVar(&values, "value", nil, "shared named value as name=value; repeatable")
	_ = command.RegisterFlagCompletionFunc("wait", fixedCompletion("all", "any", "none"))
	_ = command.RegisterFlagCompletionFunc("on-child-failed", fixedCompletion("fail_parent", "ignore"))
	_ = command.RegisterFlagCompletionFunc("on-parent-closed", fixedCompletion("cancel_children", "abandon_children"))
	return command
}

func fixedCompletion(values ...string) cobra.CompletionFunc {
	return func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return values, cobra.ShellCompDirectiveNoFileComp
	}
}

func newWorkflowStartAndClaimCommand(dependencies dependencies) *cobra.Command {
	var (
		worker       string
		initialState string
		lease        time.Duration
		partition    string
		parent       string
		root         string
		correlation  string
		priority     int64
		retention    time.Duration
		maxActive    string
		attributes   []string
		input        valueInput
	)
	command := &cobra.Command{
		Use:   "start-and-claim <type> <id> [payload]",
		Short: "Create a workflow and claim it atomically",
		Long:  "Create a workflow and immediately enter running state with a fresh lease and fencing token.",
		Example: "  ferric workflow start-and-claim order order-42 --worker worker-1\n" +
			"  ferric workflow start-and-claim order order-42 '{\"total\":125}' --json --worker worker-1 --lease 1m",
		Args: cobra.RangeArgs(2, 3),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(worker) == "" {
				return errors.New("worker is required; use --worker <name>")
			}
			if strings.TrimSpace(initialState) == "" {
				return errors.New("initial-state is required")
			}
			leaseMS, err := positiveMilliseconds("lease", lease)
			if err != nil {
				return err
			}
			var positional *string
			if len(args) == 3 {
				positional = &args[2]
			}
			payload, present, err := input.read(command, positional)
			if err != nil {
				return err
			}
			attributeValues, err := parseAssignments(attributes, "attribute")
			if err != nil {
				return err
			}
			parsedMaxActive, err := parseMaxActive(maxActive)
			if err != nil {
				return err
			}
			options := ferricstore.StartAndClaimOptions{
				ID:            args[1],
				Type:          args[0],
				InitialState:  initialState,
				Worker:        worker,
				LeaseMS:       leaseMS,
				PartitionKey:  partition,
				ParentFlowID:  parent,
				RootFlowID:    root,
				CorrelationID: correlation,
				MaxActiveMS:   parsedMaxActive,
				Attributes:    attributeValues,
			}
			if present {
				options.Payload = payload
			}
			if command.Flags().Changed("priority") {
				options.Priority = ferricstore.Int64(priority)
			}
			if command.Flags().Changed("retention") {
				value, err := positiveMilliseconds("retention", retention)
				if err != nil {
					return err
				}
				options.RetentionTTLMS = &value
			}
			return runNetworkCommand(command, dependencies, "start and claim workflow", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				starter, err := requireClientCapability[workflowStartAndClaimer](client, "FerricFlow start-and-claim")
				if err != nil {
					return nil, err
				}
				record, err := starter.StartAndClaim(ctx, options)
				return flowRecordOutput(record), err
			})
		},
	}
	command.Flags().StringVar(&worker, "worker", "", "worker receiving the initial lease (required)")
	command.Flags().StringVar(&initialState, "initial-state", "queued", "state to create before atomically entering running")
	command.Flags().DurationVar(&lease, "lease", 30*time.Second, "initial lease duration")
	command.Flags().StringVar(&partition, "partition", "", "partition key")
	command.Flags().StringVar(&parent, "parent", "", "parent workflow ID")
	command.Flags().StringVar(&root, "root", "", "root workflow ID")
	command.Flags().StringVar(&correlation, "correlation-id", "", "application correlation ID")
	command.Flags().Int64Var(&priority, "priority", 0, "scheduling priority")
	command.Flags().DurationVar(&retention, "retention", 0, "retention time after reaching a terminal state")
	command.Flags().StringVar(&maxActive, "max-active", "", "maximum active time, such as 5m or infinity")
	command.Flags().StringArrayVar(&attributes, "attribute", nil, "searchable attribute as name=value; repeatable")
	input.addFlags(command, "payload")
	_ = command.RegisterFlagCompletionFunc("initial-state", completeCommonFlowState)
	return command
}

type workflowStepContinuer interface {
	StepContinue(context.Context, ferricstore.StepContinueOptions) (*ferricstore.FlowRecord, error)
}

func newWorkflowStepContinueCommand(dependencies dependencies) *cobra.Command {
	var (
		leaseFlags  flowLeaseFlags
		lease       time.Duration
		worker      string
		attributes  []string
		deleteAttrs []string
		values      []string
		input       valueInput
	)
	command := &cobra.Command{
		Use:   "step-continue <id> <from-state> <to-state> [payload]",
		Short: "Transition a claimed workflow and renew its lease",
		Long:  "Atomically transition a claimed workflow to its next step while retaining ownership under a renewed lease.",
		Example: "  ferric workflow step-continue order-42 charging shipping --lease-token TOKEN --fencing-token 7\n" +
			"  ferric workflow step-continue order-42 charging shipping --lease-token TOKEN --fencing-token 7 --lease 1m --worker worker-2",
		Args: cobra.RangeArgs(3, 4),
		RunE: func(command *cobra.Command, args []string) error {
			if err := leaseFlags.validate(true, true); err != nil {
				return err
			}
			leaseMS, err := positiveMilliseconds("lease", lease)
			if err != nil {
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
			attributeValues, err := parseAssignments(attributes, "attribute")
			if err != nil {
				return err
			}
			namedValues, err := parseAssignments(values, "value")
			if err != nil {
				return err
			}
			options := ferricstore.StepContinueOptions{
				ID:           args[0],
				FromState:    args[1],
				ToState:      args[2],
				LeaseToken:   leaseFlags.leaseToken,
				FencingToken: leaseFlags.fencingToken,
				PartitionKey: leaseFlags.partition,
				LeaseMS:      leaseMS,
				Worker:       worker,
				NamedValues: ferricstore.NamedValues{
					Values:           namedValues,
					AttributesMerge:  attributeValues,
					AttributesDelete: deleteAttrs,
				},
			}
			if present {
				options.Payload = payload
			}
			return runNetworkCommand(command, dependencies, "continue workflow step", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				continuer, err := requireClientCapability[workflowStepContinuer](client, "FerricFlow step continuation")
				if err != nil {
					return nil, err
				}
				record, err := continuer.StepContinue(ctx, options)
				return flowRecordOutput(record), err
			})
		},
	}
	leaseFlags.add(command, true, true)
	command.Flags().DurationVar(&lease, "lease", 30*time.Second, "renewed lease duration")
	command.Flags().StringVar(&worker, "worker", "", "worker identity retained for the next step")
	command.Flags().StringArrayVar(&attributes, "attribute", nil, "attribute to merge as name=value; repeatable")
	command.Flags().StringArrayVar(&deleteAttrs, "delete-attribute", nil, "attribute to remove; repeatable")
	command.Flags().StringArrayVar(&values, "value", nil, "named value to publish as name=value; repeatable")
	input.addFlags(command, "payload")
	_ = command.RegisterFlagCompletionFunc("from-state", completeCommonFlowState)
	_ = command.RegisterFlagCompletionFunc("to-state", completeCommonFlowState)
	return command
}
