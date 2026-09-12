package cli

import (
	"context"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	ferricstore "github.com/ferricstore/ferricstore-go"
	"github.com/spf13/cobra"
)

type createItemInput struct {
	ID           string            `json:"id"`
	Payload      any               `json:"payload,omitempty"`
	PartitionKey string            `json:"partition_key,omitempty"`
	Values       map[string]any    `json:"values,omitempty"`
	ValueRefs    map[string]string `json:"value_refs,omitempty"`
	Attributes   map[string]any    `json:"attributes,omitempty"`
	StateMeta    map[string]any    `json:"state_meta,omitempty"`
	MaxActiveMS  any               `json:"max_active_ms,omitempty"`
}

type createManyInput struct {
	PartitionKey   string            `json:"partition_key,omitempty"`
	Items          []createItemInput `json:"items"`
	Type           string            `json:"type"`
	State          string            `json:"state,omitempty"`
	RunAtMS        int64             `json:"run_at_ms,omitempty"`
	NowMS          int64             `json:"now_ms,omitempty"`
	Priority       *int64            `json:"priority,omitempty"`
	Idempotent     *bool             `json:"idempotent,omitempty"`
	Independent    *bool             `json:"independent,omitempty"`
	RetentionTTLMS *int64            `json:"retention_ttl_ms,omitempty"`
	MaxActiveMS    any               `json:"max_active_ms,omitempty"`
	Values         map[string]any    `json:"values,omitempty"`
	ValueRefs      map[string]string `json:"value_refs,omitempty"`
	Attributes     map[string]any    `json:"attributes,omitempty"`
	StateMeta      map[string]any    `json:"state_meta,omitempty"`
}

func (input createManyInput) options() ferricstore.CreateManyOptions {
	items := make([]ferricstore.CreateItem, len(input.Items))
	for index, item := range input.Items {
		items[index] = ferricstore.CreateItem{
			ID:           item.ID,
			Payload:      item.Payload,
			PartitionKey: item.PartitionKey,
			Values:       item.Values,
			ValueRefs:    item.ValueRefs,
			Attributes:   item.Attributes,
			StateMeta:    item.StateMeta,
			MaxActiveMS:  item.MaxActiveMS,
		}
	}
	return ferricstore.CreateManyOptions{
		PartitionKey:   input.PartitionKey,
		Items:          items,
		Type:           input.Type,
		State:          input.State,
		RunAtMS:        input.RunAtMS,
		NowMS:          input.NowMS,
		Priority:       input.Priority,
		Idempotent:     input.Idempotent,
		Independent:    input.Independent,
		RetentionTTLMS: input.RetentionTTLMS,
		MaxActiveMS:    input.MaxActiveMS,
		Values:         input.Values,
		ValueRefs:      input.ValueRefs,
		Attributes:     input.Attributes,
		StateMeta:      input.StateMeta,
	}
}

type flowManyCreator interface {
	CreateMany(context.Context, ferricstore.CreateManyOptions) ([]ferricstore.FlowRecord, error)
}

type flowManyEnqueuer interface {
	EnqueueMany(context.Context, ferricstore.CreateManyOptions) ([]ferricstore.FlowRecord, error)
}

func newFlowCreateManyCommand(dependencies dependencies, service string) *cobra.Command {
	var path string
	name, action := "start-many", "start workflows"
	if service == "queue" {
		name, action = "enqueue-many", "enqueue jobs"
	}
	command := &cobra.Command{
		Use:   name,
		Short: action + " from one bounded JSON request",
		Long:  "Read a snake_case JSON CreateMany request from a file or standard input and submit it atomically.",
		Example: "  ferric " + service + " " + name + " --file batch.json\n" +
			"  ferric " + service + " " + name + " --file -",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			input, err := readBoundedJSONFile[createManyInput](command, path, name+" request")
			if err != nil {
				return err
			}
			if err := validateCreateManyInput(input); err != nil {
				return err
			}
			options := input.options()
			return runNetworkCommand(command, dependencies, action, func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				if service == "queue" {
					enqueuer, err := requireClientCapability[flowManyEnqueuer](client, "FerricFlow batch enqueue")
					if err != nil {
						return nil, err
					}
					records, err := enqueuer.EnqueueMany(ctx, options)
					return flowRecordsOutput(records), err
				}
				creator, err := requireClientCapability[flowManyCreator](client, "FerricFlow batch creation")
				if err != nil {
					return nil, err
				}
				records, err := creator.CreateMany(ctx, options)
				return flowRecordsOutput(records), err
			})
		},
	}
	command.Flags().StringVar(&path, "file", "", "JSON batch request; use - for stdin (required)")
	return command
}

type claimedItemInput struct {
	ID           string `json:"id"`
	LeaseToken   string `json:"lease_token"`
	FencingToken int64  `json:"fencing_token"`
	PartitionKey string `json:"partition_key,omitempty"`
}

func claimedItems(inputs []claimedItemInput) []ferricstore.ClaimedItem {
	items := make([]ferricstore.ClaimedItem, len(inputs))
	for index, item := range inputs {
		items[index] = ferricstore.ClaimedItem{
			ID:           item.ID,
			LeaseToken:   item.LeaseToken,
			FencingToken: item.FencingToken,
			PartitionKey: item.PartitionKey,
		}
	}
	return items
}

type fencedItemInput struct {
	ID           string `json:"id"`
	FencingToken int64  `json:"fencing_token"`
	LeaseToken   string `json:"lease_token,omitempty"`
	PartitionKey string `json:"partition_key,omitempty"`
}

func fencedItems(inputs []fencedItemInput) []ferricstore.FencedItem {
	items := make([]ferricstore.FencedItem, len(inputs))
	for index, item := range inputs {
		items[index] = ferricstore.FencedItem{
			ID:           item.ID,
			FencingToken: item.FencingToken,
			LeaseToken:   item.LeaseToken,
			PartitionKey: item.PartitionKey,
		}
	}
	return items
}

type namedValuesInput struct {
	Values           map[string]any    `json:"values,omitempty"`
	ValueRefs        map[string]string `json:"value_refs,omitempty"`
	DropValues       []string          `json:"drop_values,omitempty"`
	OverrideValues   []string          `json:"override_values,omitempty"`
	AttributesMerge  map[string]any    `json:"attributes_merge,omitempty"`
	AttributesDelete []string          `json:"attributes_delete,omitempty"`
}

func (input namedValuesInput) options() ferricstore.NamedValues {
	return ferricstore.NamedValues{
		Values:           input.Values,
		ValueRefs:        input.ValueRefs,
		DropValues:       input.DropValues,
		OverrideValues:   input.OverrideValues,
		AttributesMerge:  input.AttributesMerge,
		AttributesDelete: input.AttributesDelete,
	}
}

type completeManyInput struct {
	namedValuesInput
	PartitionKey string             `json:"partition_key,omitempty"`
	Items        []claimedItemInput `json:"items"`
	Result       any                `json:"result,omitempty"`
	Payload      any                `json:"payload,omitempty"`
	TTLMS        *int64             `json:"ttl_ms,omitempty"`
	NowMS        int64              `json:"now_ms,omitempty"`
	Independent  *bool              `json:"independent,omitempty"`
	StateMeta    map[string]any     `json:"state_meta,omitempty"`
}

type flowManyCompleter interface {
	CompleteMany(context.Context, ferricstore.CompleteManyOptions) ([]ferricstore.FlowRecord, error)
}

func newFlowCompleteManyCommand(dependencies dependencies, service string) *cobra.Command {
	var path string
	command := &cobra.Command{
		Use:     "complete-many",
		Short:   "Complete multiple claimed executions from JSON",
		Example: "  ferric " + service + " complete-many --file completions.json",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			input, err := readBoundedJSONFile[completeManyInput](command, path, "complete-many request")
			if err != nil {
				return err
			}
			if err := validateClaimedItemsInput(input.Items, input.Independent); err != nil {
				return err
			}
			options := ferricstore.CompleteManyOptions{
				PartitionKey: input.PartitionKey,
				Items:        claimedItems(input.Items),
				Result:       input.Result,
				Payload:      input.Payload,
				TTLMS:        input.TTLMS,
				NowMS:        input.NowMS,
				Independent:  input.Independent,
				StateMeta:    input.StateMeta,
				NamedValues:  input.options(),
			}
			return runNetworkCommand(command, dependencies, "complete "+service+" batch", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				completer, err := requireClientCapability[flowManyCompleter](client, "FerricFlow batch completion")
				if err != nil {
					return nil, err
				}
				records, err := completer.CompleteMany(ctx, options)
				return flowRecordsOutput(records), err
			})
		},
	}
	command.Flags().StringVar(&path, "file", "", "JSON batch request; use - for stdin (required)")
	return command
}

type transitionManyInput struct {
	namedValuesInput
	PartitionKey string            `json:"partition_key,omitempty"`
	FromState    string            `json:"from_state"`
	ToState      string            `json:"to_state"`
	Items        []fencedItemInput `json:"items"`
	Payload      any               `json:"payload,omitempty"`
	RunAtMS      int64             `json:"run_at_ms,omitempty"`
	NowMS        int64             `json:"now_ms,omitempty"`
	Priority     *int64            `json:"priority,omitempty"`
	Independent  *bool             `json:"independent,omitempty"`
	StateMeta    map[string]any    `json:"state_meta,omitempty"`
}

type flowManyTransitioner interface {
	TransitionMany(context.Context, ferricstore.TransitionManyOptions) ([]ferricstore.FlowRecord, error)
}

func newFlowTransitionManyCommand(dependencies dependencies) *cobra.Command {
	var path string
	command := &cobra.Command{
		Use:     "transition-many",
		Short:   "Transition multiple fenced workflows from JSON",
		Example: "  ferric workflow transition-many --file transitions.json",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			input, err := readBoundedJSONFile[transitionManyInput](command, path, "transition-many request")
			if err != nil {
				return err
			}
			if err := validateTransitionManyInput(input); err != nil {
				return err
			}
			options := ferricstore.TransitionManyOptions{
				PartitionKey: input.PartitionKey,
				FromState:    input.FromState,
				ToState:      input.ToState,
				Items:        fencedItems(input.Items),
				Payload:      input.Payload,
				RunAtMS:      input.RunAtMS,
				NowMS:        input.NowMS,
				Priority:     input.Priority,
				Independent:  input.Independent,
				StateMeta:    input.StateMeta,
				NamedValues:  input.options(),
			}
			return runNetworkCommand(command, dependencies, "transition workflow batch", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				transitioner, err := requireClientCapability[flowManyTransitioner](client, "FerricFlow batch transition")
				if err != nil {
					return nil, err
				}
				records, err := transitioner.TransitionMany(ctx, options)
				return flowRecordsOutput(records), err
			})
		},
	}
	command.Flags().StringVar(&path, "file", "", "JSON batch request; use - for stdin (required)")
	return command
}

type retryManyInput struct {
	PartitionKey     string             `json:"partition_key,omitempty"`
	Items            []claimedItemInput `json:"items"`
	Error            any                `json:"error,omitempty"`
	Payload          any                `json:"payload,omitempty"`
	RunAtMS          int64              `json:"run_at_ms,omitempty"`
	NowMS            int64              `json:"now_ms,omitempty"`
	Independent      *bool              `json:"independent,omitempty"`
	StateMeta        map[string]any     `json:"state_meta,omitempty"`
	AttributesMerge  map[string]any     `json:"attributes_merge,omitempty"`
	AttributesDelete []string           `json:"attributes_delete,omitempty"`
}

type flowManyRetrier interface {
	RetryMany(context.Context, ferricstore.RetryManyOptions) ([]ferricstore.FlowRecord, error)
}

func newFlowRetryManyCommand(dependencies dependencies, service string) *cobra.Command {
	var path string
	command := &cobra.Command{
		Use:     "retry-many",
		Short:   "Retry multiple claimed executions from JSON",
		Example: "  ferric " + service + " retry-many --file retries.json",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			input, err := readBoundedJSONFile[retryManyInput](command, path, "retry-many request")
			if err != nil {
				return err
			}
			if err := validateClaimedItemsInput(input.Items, input.Independent); err != nil {
				return err
			}
			options := ferricstore.RetryManyOptions{
				PartitionKey:     input.PartitionKey,
				Items:            claimedItems(input.Items),
				Error:            input.Error,
				Payload:          input.Payload,
				RunAtMS:          input.RunAtMS,
				NowMS:            input.NowMS,
				Independent:      input.Independent,
				StateMeta:        input.StateMeta,
				AttributesMerge:  input.AttributesMerge,
				AttributesDelete: input.AttributesDelete,
			}
			return runNetworkCommand(command, dependencies, "retry "+service+" batch", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				retrier, err := requireClientCapability[flowManyRetrier](client, "FerricFlow batch retry")
				if err != nil {
					return nil, err
				}
				records, err := retrier.RetryMany(ctx, options)
				return flowRecordsOutput(records), err
			})
		},
	}
	command.Flags().StringVar(&path, "file", "", "JSON batch request; use - for stdin (required)")
	return command
}

type failManyInput struct {
	namedValuesInput
	PartitionKey string             `json:"partition_key,omitempty"`
	Items        []claimedItemInput `json:"items"`
	Error        any                `json:"error,omitempty"`
	Payload      any                `json:"payload,omitempty"`
	TTLMS        *int64             `json:"ttl_ms,omitempty"`
	NowMS        int64              `json:"now_ms,omitempty"`
	Independent  *bool              `json:"independent,omitempty"`
	StateMeta    map[string]any     `json:"state_meta,omitempty"`
}

type flowManyFailer interface {
	FailMany(context.Context, ferricstore.FailManyOptions) ([]ferricstore.FlowRecord, error)
}

func newFlowFailManyCommand(dependencies dependencies, service string) *cobra.Command {
	var path string
	command := &cobra.Command{
		Use:     "fail-many",
		Short:   "Fail multiple claimed executions from JSON",
		Example: "  ferric " + service + " fail-many --file failures.json",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			input, err := readBoundedJSONFile[failManyInput](command, path, "fail-many request")
			if err != nil {
				return err
			}
			if err := validateClaimedItemsInput(input.Items, input.Independent); err != nil {
				return err
			}
			options := ferricstore.FailManyOptions{
				PartitionKey: input.PartitionKey,
				Items:        claimedItems(input.Items),
				Error:        input.Error,
				Payload:      input.Payload,
				TTLMS:        input.TTLMS,
				NowMS:        input.NowMS,
				Independent:  input.Independent,
				StateMeta:    input.StateMeta,
				NamedValues:  input.options(),
			}
			return runNetworkCommand(command, dependencies, "fail "+service+" batch", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				failer, err := requireClientCapability[flowManyFailer](client, "FerricFlow batch failure")
				if err != nil {
					return nil, err
				}
				records, err := failer.FailMany(ctx, options)
				return flowRecordsOutput(records), err
			})
		},
	}
	command.Flags().StringVar(&path, "file", "", "JSON batch request; use - for stdin (required)")
	return command
}

type cancelManyInput struct {
	namedValuesInput
	PartitionKey string            `json:"partition_key,omitempty"`
	Items        []fencedItemInput `json:"items"`
	Reason       any               `json:"reason,omitempty"`
	TTLMS        *int64            `json:"ttl_ms,omitempty"`
	NowMS        int64             `json:"now_ms,omitempty"`
	Independent  *bool             `json:"independent,omitempty"`
	StateMeta    map[string]any    `json:"state_meta,omitempty"`
}

type flowManyCanceller interface {
	CancelMany(context.Context, ferricstore.CancelManyOptions) ([]ferricstore.FlowRecord, error)
}

func newFlowCancelManyCommand(dependencies dependencies, service string) *cobra.Command {
	var path string
	command := &cobra.Command{
		Use:     "cancel-many",
		Short:   "Cancel multiple fenced executions from JSON",
		Example: "  ferric " + service + " cancel-many --file cancellations.json",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			input, err := readBoundedJSONFile[cancelManyInput](command, path, "cancel-many request")
			if err != nil {
				return err
			}
			if err := validateFencedItemsInput(input.Items, input.Independent, false); err != nil {
				return err
			}
			options := ferricstore.CancelManyOptions{
				PartitionKey: input.PartitionKey,
				Items:        fencedItems(input.Items),
				Reason:       input.Reason,
				TTLMS:        input.TTLMS,
				NowMS:        input.NowMS,
				Independent:  input.Independent,
				StateMeta:    input.StateMeta,
				NamedValues:  input.options(),
			}
			return runNetworkCommand(command, dependencies, "cancel "+service+" batch", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				canceller, err := requireClientCapability[flowManyCanceller](client, "FerricFlow batch cancellation")
				if err != nil {
					return nil, err
				}
				records, err := canceller.CancelMany(ctx, options)
				return flowRecordsOutput(records), err
			})
		},
	}
	command.Flags().StringVar(&path, "file", "", "JSON batch request; use - for stdin (required)")
	return command
}

type runStepsItemInput struct {
	ID           string `json:"id"`
	PartitionKey string `json:"partition_key,omitempty"`
}

type runStepsManyInput struct {
	Items          []runStepsItemInput `json:"items"`
	Type           string              `json:"type"`
	States         []string            `json:"states"`
	Steps          int                 `json:"steps"`
	Worker         string              `json:"worker"`
	LeaseMS        int64               `json:"lease_ms"`
	NowMS          int64               `json:"now_ms,omitempty"`
	Payload        any                 `json:"payload,omitempty"`
	Result         any                 `json:"result,omitempty"`
	PartitionKey   string              `json:"partition_key,omitempty"`
	RetentionTTLMS *int64              `json:"retention_ttl_ms,omitempty"`
}

type flowManyStepRunner interface {
	RunStepsMany(context.Context, ferricstore.RunStepsManyOptions) error
}

func newWorkflowRunStepsManyCommand(dependencies dependencies) *cobra.Command {
	var path string
	command := &cobra.Command{
		Use:     "run-steps-many",
		Short:   "Run multiple bounded workflow steps from JSON",
		Example: "  ferric workflow run-steps-many --file steps.json",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			input, err := readBoundedJSONFile[runStepsManyInput](command, path, "run-steps-many request")
			if err != nil {
				return err
			}
			if err := validateRunStepsManyInput(input); err != nil {
				return err
			}
			items := make([]ferricstore.RunStepsItem, len(input.Items))
			for index, item := range input.Items {
				items[index] = ferricstore.RunStepsItem{ID: item.ID, PartitionKey: item.PartitionKey}
			}
			options := ferricstore.RunStepsManyOptions{
				Items:          items,
				Type:           input.Type,
				States:         input.States,
				Steps:          input.Steps,
				Worker:         input.Worker,
				LeaseMS:        input.LeaseMS,
				NowMS:          input.NowMS,
				Payload:        input.Payload,
				Result:         input.Result,
				PartitionKey:   input.PartitionKey,
				RetentionTTLMS: input.RetentionTTLMS,
			}
			return runNetworkCommand(command, dependencies, "run workflow steps batch", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				runner, err := requireClientCapability[flowManyStepRunner](client, "FerricFlow batch steps")
				if err != nil {
					return nil, err
				}
				err = runner.RunStepsMany(ctx, options)
				return map[string]any{"processed": len(options.Items)}, err
			})
		},
	}
	command.Flags().StringVar(&path, "file", "", "JSON batch request; use - for stdin (required)")
	return command
}
