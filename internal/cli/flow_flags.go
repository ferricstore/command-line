package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	ferricstore "github.com/ferricstore/ferricstore-go"
	"github.com/spf13/cobra"
)

type flowCreateFlags struct {
	state       string
	partition   string
	runAt       string
	delay       time.Duration
	priority    int64
	idempotent  bool
	retention   time.Duration
	maxActive   string
	attributes  []string
	payload     valueInput
	parent      string
	root        string
	correlation string
}

func (flags *flowCreateFlags) add(command *cobra.Command, workflow bool) {
	command.Flags().StringVar(&flags.state, "state", "queued", "initial state")
	command.Flags().StringVar(&flags.partition, "partition", "", "partition key")
	command.Flags().StringVar(&flags.runAt, "run-at", "", "RFC3339 time or Unix milliseconds at which work becomes due")
	command.Flags().DurationVar(&flags.delay, "delay", 0, "make work due after this duration")
	command.Flags().Int64Var(&flags.priority, "priority", 0, "scheduling priority")
	command.Flags().BoolVar(&flags.idempotent, "idempotent", false, "return the existing execution when the ID already exists")
	command.Flags().DurationVar(&flags.retention, "retention", 0, "retention time after reaching a terminal state")
	command.Flags().StringVar(&flags.maxActive, "max-active", "", "maximum active time, such as 5m or infinity")
	command.Flags().StringArrayVar(&flags.attributes, "attribute", nil, "searchable attribute as name=value; repeatable")
	if workflow {
		command.Flags().StringVar(&flags.parent, "parent", "", "parent workflow ID")
		command.Flags().StringVar(&flags.root, "root", "", "root workflow ID")
		command.Flags().StringVar(&flags.correlation, "correlation-id", "", "application correlation ID")
	}
	flags.payload.addFlags(command, "payload")
	_ = command.RegisterFlagCompletionFunc("state", completeCommonFlowState)
}

func (flags flowCreateFlags) options(command *cobra.Command, flowType, id string, payloadArg *string) (ferricstore.CreateOptions, error) {
	payload, present, err := flags.payload.read(command, payloadArg)
	if err != nil {
		return ferricstore.CreateOptions{}, err
	}
	runAt, err := parseRunAt(flags.runAt, flags.delay, time.Now())
	if err != nil {
		return ferricstore.CreateOptions{}, err
	}
	attributes, err := parseAssignments(flags.attributes, "attribute")
	if err != nil {
		return ferricstore.CreateOptions{}, err
	}
	maxActive, err := parseMaxActive(flags.maxActive)
	if err != nil {
		return ferricstore.CreateOptions{}, err
	}
	options := ferricstore.CreateOptions{
		ID:            id,
		Type:          flowType,
		State:         flags.state,
		PartitionKey:  flags.partition,
		RunAtMS:       runAt,
		MaxActiveMS:   maxActive,
		Attributes:    attributes,
		ParentFlowID:  flags.parent,
		RootFlowID:    flags.root,
		CorrelationID: flags.correlation,
		ReturnRecord:  true,
	}
	if present {
		options.Payload = payload
	}
	if command.Flags().Changed("priority") {
		options.Priority = ferricstore.Int64(flags.priority)
	}
	if command.Flags().Changed("idempotent") {
		options.Idempotent = ferricstore.Bool(flags.idempotent)
	}
	if command.Flags().Changed("retention") {
		value, err := positiveMilliseconds("retention", flags.retention)
		if err != nil {
			return ferricstore.CreateOptions{}, err
		}
		options.RetentionTTLMS = &value
	}
	return options, nil
}

type flowReadFlags struct {
	partition            string
	state                string
	limit                int
	reverse              bool
	terminalOnly         bool
	includeCold          bool
	consistentProjection bool
	attributes           []string
}

func (flags *flowReadFlags) add(command *cobra.Command, includeState bool) {
	command.Flags().StringVar(&flags.partition, "partition", "", "partition key")
	if includeState {
		command.Flags().StringVar(&flags.state, "state", "", "filter by state")
		_ = command.RegisterFlagCompletionFunc("state", completeCommonFlowState)
	}
	command.Flags().IntVar(&flags.limit, "limit", 100, "maximum records to return")
	command.Flags().BoolVar(&flags.reverse, "reverse", false, "return newest records first")
	command.Flags().BoolVar(&flags.terminalOnly, "terminal-only", false, "return only terminal records")
	command.Flags().BoolVar(&flags.includeCold, "include-cold", false, "include retained cold records")
	command.Flags().BoolVar(&flags.consistentProjection, "consistent", false, "wait for a consistent cold projection")
	command.Flags().StringArrayVar(&flags.attributes, "attribute", nil, "filter attribute as name=value; repeatable")
}

func (flags flowReadFlags) options(command *cobra.Command) (ferricstore.ReadOptions, error) {
	if flags.limit <= 0 {
		return ferricstore.ReadOptions{}, errors.New("limit must be greater than zero")
	}
	attributes, err := parseAssignments(flags.attributes, "attribute")
	if err != nil {
		return ferricstore.ReadOptions{}, err
	}
	options := ferricstore.ReadOptions{
		PartitionKey: flags.partition,
		State:        flags.state,
		Count:        ferricstore.Int(flags.limit),
		Attributes:   attributes,
	}
	if command.Flags().Changed("reverse") {
		options.Rev = ferricstore.Bool(flags.reverse)
	}
	if command.Flags().Changed("terminal-only") {
		options.TerminalOnly = ferricstore.Bool(flags.terminalOnly)
	}
	if command.Flags().Changed("include-cold") {
		options.IncludeCold = ferricstore.Bool(flags.includeCold)
	}
	if command.Flags().Changed("consistent") {
		options.ConsistentProjection = ferricstore.Bool(flags.consistentProjection)
	}
	return options, nil
}

type flowClaimFlags struct {
	states            []string
	worker            string
	partition         string
	lease             time.Duration
	limit             int
	block             time.Duration
	reclaimExpired    bool
	payload           bool
	includeAttributes bool
	values            []string
}

func (flags *flowClaimFlags) add(command *cobra.Command) {
	command.Flags().StringSliceVar(&flags.states, "state", []string{"queued"}, "eligible state; repeatable")
	command.Flags().StringVar(&flags.worker, "worker", "", "worker identity (required)")
	command.Flags().StringVar(&flags.partition, "partition", "", "partition key")
	command.Flags().DurationVar(&flags.lease, "lease", 30*time.Second, "lease duration")
	command.Flags().IntVar(&flags.limit, "limit", 1, "maximum jobs to claim")
	command.Flags().DurationVar(&flags.block, "wait", 0, "wait for due work for up to this duration")
	command.Flags().BoolVar(&flags.reclaimExpired, "reclaim-expired", false, "include work with an expired lease")
	command.Flags().BoolVar(&flags.payload, "payload", true, "include payloads in claimed jobs")
	command.Flags().BoolVar(&flags.includeAttributes, "attributes", true, "include attributes in claimed jobs")
	command.Flags().StringSliceVar(&flags.values, "value", nil, "named value to include; repeatable")
	_ = command.RegisterFlagCompletionFunc("state", completeCommonFlowState)
}

func (flags flowClaimFlags) options(command *cobra.Command, flowType string) (ferricstore.ClaimDueOptions, error) {
	if strings.TrimSpace(flags.worker) == "" {
		return ferricstore.ClaimDueOptions{}, errors.New("worker is required; use --worker <name>")
	}
	leaseMS, err := positiveMilliseconds("lease", flags.lease)
	if err != nil {
		return ferricstore.ClaimDueOptions{}, err
	}
	if flags.limit <= 0 {
		return ferricstore.ClaimDueOptions{}, errors.New("limit must be greater than zero")
	}
	if flags.block < 0 {
		return ferricstore.ClaimDueOptions{}, errors.New("wait must not be negative")
	}
	options := ferricstore.ClaimDueOptions{
		Type:              flowType,
		States:            flags.states,
		Worker:            flags.worker,
		PartitionKey:      flags.partition,
		LeaseMS:           leaseMS,
		Limit:             flags.limit,
		JobOnly:           true,
		Payload:           ferricstore.Bool(flags.payload),
		Values:            flags.values,
		IncludeState:      true,
		IncludeAttributes: ferricstore.Bool(flags.includeAttributes),
	}
	if flags.block > 0 {
		blockMS := flags.block.Milliseconds()
		options.BlockMS = &blockMS
	}
	if command.Flags().Changed("reclaim-expired") {
		options.ReclaimExpired = ferricstore.Bool(flags.reclaimExpired)
	}
	return options, nil
}

type flowLeaseFlags struct {
	partition    string
	leaseToken   string
	fencingToken int64
}

func (flags *flowLeaseFlags) add(command *cobra.Command, requireLease, requirePositiveFencing bool) {
	command.Flags().StringVar(&flags.partition, "partition", "", "partition key")
	command.Flags().StringVar(&flags.leaseToken, "lease-token", "", "lease token returned by claim")
	command.Flags().Int64Var(&flags.fencingToken, "fencing-token", 0, "fencing token returned by claim")
	if requireLease {
		command.Flags().Lookup("lease-token").Usage += " (required)"
	}
	if requirePositiveFencing {
		command.Flags().Lookup("fencing-token").Usage += " (required)"
	}
}

func (flags flowLeaseFlags) validate(requireLease, requirePositiveFencing bool) error {
	if requireLease && strings.TrimSpace(flags.leaseToken) == "" {
		return errors.New("lease token is required; use --lease-token")
	}
	if requirePositiveFencing && flags.fencingToken <= 0 {
		return errors.New("fencing token must be greater than zero; use --fencing-token")
	}
	if flags.fencingToken < 0 {
		return errors.New("fencing token must not be negative")
	}
	return nil
}

func parseRunAt(value string, delay time.Duration, now time.Time) (int64, error) {
	value = strings.TrimSpace(value)
	if value != "" && delay != 0 {
		return 0, errors.New("use --run-at or --delay, not both")
	}
	if delay < 0 {
		return 0, errors.New("delay must not be negative")
	}
	if delay > 0 {
		return now.Add(delay).UnixMilli(), nil
	}
	if value == "" {
		return 0, nil
	}
	if milliseconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if milliseconds <= 0 {
			return 0, errors.New("run-at Unix milliseconds must be greater than zero")
		}
		return milliseconds, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0, fmt.Errorf("invalid run-at %q: use RFC3339 or Unix milliseconds", value)
	}
	return parsed.UnixMilli(), nil
}

func parseMaxActive(value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if strings.EqualFold(value, "infinity") {
		return "INFINITY", nil
	}
	if milliseconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if milliseconds <= 0 {
			return nil, errors.New("max-active milliseconds must be greater than zero")
		}
		return milliseconds, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return nil, fmt.Errorf("invalid max-active %q: use a duration such as 5m or infinity", value)
	}
	return duration.Milliseconds(), nil
}

func positiveMilliseconds(name string, duration time.Duration) (int64, error) {
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	if duration.Milliseconds() <= 0 {
		return 0, fmt.Errorf("%s must be at least 1ms", name)
	}
	return duration.Milliseconds(), nil
}

func completeCommonFlowState(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"queued", "running", "waiting", "completed", "failed", "cancelled"}, cobra.ShellCompDirectiveNoFileComp
}
