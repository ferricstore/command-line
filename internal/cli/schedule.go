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

func newScheduleCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "schedule",
		Short: "Create and manage FerricFlow schedules",
		Long:  "Manage workflow schedule definitions independently from their one-shot, delayed, interval, or cron executions.",
		Example: "  ferric workflow schedule create daily-report --type report --cron '0 8 * * *' --timezone UTC --id-prefix daily-report\n" +
			"  ferric workflow schedule list --state active\n" +
			"  ferric workflow schedule pause daily-report",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newScheduleCreateCommand(dependencies),
		newScheduleDescribeCommand(dependencies),
		newScheduleListCommand(dependencies),
		newSchedulePauseCommand(dependencies),
		newScheduleResumeCommand(dependencies),
		newScheduleTriggerCommand(dependencies),
		newScheduleFireDueCommand(dependencies),
		newScheduleDeleteCommand(dependencies),
	)
	return command
}

type scheduleCreator interface {
	ScheduleCreate(context.Context, string, ferricstore.ScheduleOptions) (ferricstore.ScheduleRecord, error)
}

type scheduleCreateFlags struct {
	flowType     string
	flowID       string
	idPrefix     string
	state        string
	partition    string
	correlation  string
	parent       string
	root         string
	priority     int64
	values       []string
	at           string
	after        time.Duration
	every        time.Duration
	cron         string
	timezone     string
	catchup      string
	startAt      string
	endAt        string
	overlap      string
	overlapRetry time.Duration
	maxFires     int64
	overwrite    bool
	payload      valueInput
}

func newScheduleCreateCommand(dependencies dependencies) *cobra.Command {
	var flags scheduleCreateFlags
	command := &cobra.Command{
		Use:   "create <id> [payload]",
		Short: "Create a one-shot or recurring schedule",
		Long: "Create one schedule target. Choose at most one of --at, --after, --every, or --cron. " +
			"Recurring schedules may use --id-prefix; fixed --flow-id is for one-shot schedules.",
		Example: "  ferric workflow schedule create welcome-42 '{\"user\":42}' --json --type welcome-email --flow-id welcome-42 --after 5m\n" +
			"  ferric workflow schedule create daily-report --type report --id-prefix report-daily --cron '0 8 * * *' --timezone UTC --overlap skip",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(command *cobra.Command, args []string) error {
			options, err := flags.options(command, args)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "create schedule", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				creator, err := requireClientCapability[scheduleCreator](client, "FerricFlow schedule creation")
				if err != nil {
					return nil, err
				}
				result, err := creator.ScheduleCreate(ctx, args[0], options)
				return scheduleResultOutput(&result), err
			})
		},
	}
	command.Flags().StringVar(&flags.flowType, "type", "", "target workflow type (required)")
	command.Flags().StringVar(&flags.flowID, "flow-id", "", "fixed target workflow ID for a one-shot schedule")
	command.Flags().StringVar(&flags.idPrefix, "id-prefix", "", "target ID prefix for a recurring schedule")
	command.Flags().StringVar(&flags.state, "state", "queued", "target initial state")
	command.Flags().StringVar(&flags.partition, "partition", "", "target partition key")
	command.Flags().StringVar(&flags.correlation, "correlation-id", "", "target correlation ID")
	command.Flags().StringVar(&flags.parent, "parent", "", "target parent workflow ID")
	command.Flags().StringVar(&flags.root, "root", "", "target root workflow ID")
	command.Flags().Int64Var(&flags.priority, "priority", 0, "target priority from 0 to 2")
	command.Flags().StringArrayVar(&flags.values, "value", nil, "target named value as name=value; repeatable")
	command.Flags().StringVar(&flags.at, "at", "", "run once at RFC3339 time or Unix milliseconds")
	command.Flags().DurationVar(&flags.after, "after", 0, "run once after this delay")
	command.Flags().DurationVar(&flags.every, "every", 0, "run repeatedly at this interval")
	command.Flags().StringVar(&flags.cron, "cron", "", "run repeatedly using this cron expression")
	command.Flags().StringVar(&flags.timezone, "timezone", "", "IANA timezone for a cron schedule")
	command.Flags().StringVar(&flags.catchup, "catchup", "", "interval recovery policy: fire_once")
	command.Flags().StringVar(&flags.startAt, "start-at", "", "first recurring run at RFC3339 time or Unix milliseconds")
	command.Flags().StringVar(&flags.endAt, "end-at", "", "stop recurring runs at RFC3339 time or Unix milliseconds")
	command.Flags().StringVar(&flags.overlap, "overlap", "", "recurring overlap policy: allow, skip, queue_after_previous, or fail_schedule")
	command.Flags().DurationVar(&flags.overlapRetry, "overlap-retry", 0, "delay before retrying queued overlap")
	command.Flags().Int64Var(&flags.maxFires, "max-fires", 0, "maximum recurring executions")
	command.Flags().BoolVar(&flags.overwrite, "overwrite", false, "replace an existing schedule with the same ID")
	flags.payload.addFlags(command, "payload")
	_ = command.RegisterFlagCompletionFunc("state", completeCommonFlowState)
	_ = command.RegisterFlagCompletionFunc("overlap", completeScheduleOverlap)
	_ = command.RegisterFlagCompletionFunc("catchup", completeScheduleCatchup)
	return command
}

func (flags scheduleCreateFlags) options(command *cobra.Command, args []string) (ferricstore.ScheduleOptions, error) {
	if strings.TrimSpace(flags.flowType) == "" {
		return ferricstore.ScheduleOptions{}, errors.New("target type is required; use --type <workflow-type>")
	}
	if flags.flowID != "" && flags.idPrefix != "" {
		return ferricstore.ScheduleOptions{}, errors.New("use --flow-id or --id-prefix, not both")
	}
	var positional *string
	if len(args) == 2 {
		positional = &args[1]
	}
	payload, payloadPresent, err := flags.payload.read(command, positional)
	if err != nil {
		return ferricstore.ScheduleOptions{}, err
	}
	values, err := parseAssignments(flags.values, "value")
	if err != nil {
		return ferricstore.ScheduleOptions{}, err
	}
	target := map[string]any{"type": flags.flowType, "state": flags.state}
	putTarget(target, "id", flags.flowID)
	putTarget(target, "id_prefix", flags.idPrefix)
	putTarget(target, "partition_key", flags.partition)
	putTarget(target, "correlation_id", flags.correlation)
	putTarget(target, "parent_flow_id", flags.parent)
	putTarget(target, "root_flow_id", flags.root)
	if payloadPresent {
		target["payload"] = payload
	}
	if len(values) != 0 {
		target["values"] = values
	}
	if command.Flags().Changed("priority") {
		if flags.priority < 0 || flags.priority > 2 {
			return ferricstore.ScheduleOptions{}, errors.New("priority must be between 0 and 2")
		}
		target["priority"] = flags.priority
	}

	timingChoices := 0
	if flags.at != "" {
		timingChoices++
	}
	if flags.after != 0 {
		timingChoices++
	}
	if flags.every != 0 {
		timingChoices++
	}
	if flags.cron != "" {
		timingChoices++
	}
	if timingChoices > 1 {
		return ferricstore.ScheduleOptions{}, errors.New("use only one of --at, --after, --every, or --cron")
	}
	recurring := flags.every != 0 || flags.cron != ""
	if recurring && flags.flowID != "" {
		return ferricstore.ScheduleOptions{}, errors.New("flow-id is only valid for one-shot schedules; use --id-prefix for recurring schedules")
	}
	if !recurring && (flags.overlap != "" || command.Flags().Changed("overlap-retry")) {
		return ferricstore.ScheduleOptions{}, errors.New("overlap settings are only valid for interval or cron schedules")
	}
	if !recurring && (command.Flags().Changed("max-fires") || flags.endAt != "") {
		return ferricstore.ScheduleOptions{}, errors.New("max-fires and end-at are only valid for interval or cron schedules")
	}
	if flags.startAt != "" && (flags.at != "" || flags.after != 0) {
		return ferricstore.ScheduleOptions{}, errors.New("start-at cannot be combined with at or after")
	}
	options := ferricstore.ScheduleOptions{Target: target}
	if flags.at != "" {
		value, err := parseAbsoluteMilliseconds("at", flags.at)
		if err != nil {
			return options, err
		}
		options.Kind = "one_shot"
		options.AtMS = &value
	}
	if flags.after != 0 {
		value, err := positiveMilliseconds("after", flags.after)
		if err != nil {
			return options, err
		}
		options.Kind = "delay"
		options.DelayMS = &value
	}
	if flags.every != 0 {
		value, err := positiveMilliseconds("every", flags.every)
		if err != nil {
			return options, err
		}
		options.Kind = "interval"
		options.EveryMS = &value
	}
	if flags.cron != "" {
		options.Kind = "cron"
		options.Cron = flags.cron
		options.Timezone = flags.timezone
	} else if flags.timezone != "" {
		return options, errors.New("timezone is only valid with --cron")
	}
	if flags.catchup != "" {
		if flags.every == 0 {
			return options, errors.New("catchup is only valid with --every")
		}
		if !strings.EqualFold(strings.TrimSpace(flags.catchup), "fire_once") {
			return options, errors.New("catchup must be fire_once")
		}
		options.CatchupPolicy = "fire_once"
	}
	if flags.startAt != "" {
		value, err := parseAbsoluteMilliseconds("start-at", flags.startAt)
		if err != nil {
			return options, err
		}
		options.StartAtMS = &value
	}
	if flags.endAt != "" {
		value, err := parseAbsoluteMilliseconds("end-at", flags.endAt)
		if err != nil {
			return options, err
		}
		options.EndAtMS = &value
	}
	if flags.overlap != "" {
		normalized := strings.ToLower(strings.TrimSpace(flags.overlap))
		switch normalized {
		case "allow", "skip", "queue_after_previous", "fail_schedule":
			options.OverlapPolicy = normalized
		default:
			return options, errors.New("overlap must be allow, skip, queue_after_previous, or fail_schedule")
		}
	}
	if command.Flags().Changed("overlap-retry") {
		if !strings.EqualFold(strings.TrimSpace(flags.overlap), "queue_after_previous") {
			return options, errors.New("overlap-retry requires --overlap queue_after_previous")
		}
		value, err := positiveMilliseconds("overlap-retry", flags.overlapRetry)
		if err != nil {
			return options, err
		}
		options.OverlapRetryMS = &value
	}
	if command.Flags().Changed("max-fires") {
		if flags.maxFires <= 0 {
			return options, errors.New("max-fires must be greater than zero")
		}
		options.MaxFires = &flags.maxFires
	}
	if command.Flags().Changed("overwrite") {
		options.Overwrite = ferricstore.Bool(flags.overwrite)
	}
	return options, nil
}

func putTarget(target map[string]any, key, value string) {
	if value != "" {
		target[key] = value
	}
}

func parseAbsoluteMilliseconds(name, value string) (int64, error) {
	milliseconds, err := parseRunAt(value, 0, time.Now())
	if err != nil {
		return 0, errors.New(strings.Replace(err.Error(), "run-at", name, 1))
	}
	return milliseconds, nil
}

func completeScheduleOverlap(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"allow", "skip", "queue_after_previous", "fail_schedule"}, cobra.ShellCompDirectiveNoFileComp
}

func completeScheduleCatchup(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"fire_once"}, cobra.ShellCompDirectiveNoFileComp
}

type scheduleGetter interface {
	ScheduleGet(context.Context, string, *int64) (*ferricstore.ScheduleRecord, error)
}

func newScheduleDescribeCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "describe <id>",
		Aliases: []string{"get", "inspect"},
		Short:   "Describe one schedule",
		Example: "  ferric workflow schedule describe daily-report",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "describe schedule", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				getter, err := requireClientCapability[scheduleGetter](client, "FerricFlow schedule reads")
				if err != nil {
					return nil, err
				}
				result, err := getter.ScheduleGet(ctx, args[0], nil)
				return scheduleResultOutput(result), err
			})
		},
	}
}

type scheduleLister interface {
	ScheduleList(context.Context, ferricstore.ScheduleListOptions) ([]ferricstore.ScheduleRecord, error)
}

func newScheduleListCommand(dependencies dependencies) *cobra.Command {
	var kind string
	var state string
	var timezone string
	var targetType string
	var limit int
	var reverse bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List schedules",
		Example: "  ferric workflow schedule list\n" +
			"  ferric workflow schedule list --kind cron --state active --type report --limit 20 --reverse",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if limit <= 0 {
				return errors.New("limit must be greater than zero")
			}
			options := ferricstore.ScheduleListOptions{
				Kind:       kind,
				State:      state,
				Timezone:   timezone,
				TargetType: targetType,
				Count:      ferricstore.Int(limit),
			}
			if command.Flags().Changed("reverse") {
				options.Rev = ferricstore.Bool(reverse)
			}
			return runNetworkCommand(command, dependencies, "list schedules", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				lister, err := requireClientCapability[scheduleLister](client, "FerricFlow schedule list")
				if err != nil {
					return nil, err
				}
				results, err := lister.ScheduleList(ctx, options)
				return scheduleResultsOutput(results), err
			})
		},
	}
	command.Flags().StringVar(&kind, "kind", "", "filter by one_shot, delay, interval, or cron")
	command.Flags().StringVar(&state, "state", "", "filter by schedule state")
	command.Flags().StringVar(&timezone, "timezone", "", "filter by cron timezone")
	command.Flags().StringVar(&targetType, "type", "", "filter by target workflow type")
	command.Flags().IntVar(&limit, "limit", 100, "maximum schedules to return")
	command.Flags().BoolVar(&reverse, "reverse", false, "return newest schedules first")
	_ = command.RegisterFlagCompletionFunc("kind", completeScheduleKind)
	_ = command.RegisterFlagCompletionFunc("state", completeScheduleState)
	return command
}

func completeScheduleKind(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"one_shot", "delay", "interval", "cron"}, cobra.ShellCompDirectiveNoFileComp
}

func completeScheduleState(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"active", "paused", "running", "completed", "failed", "cancelled", "all"}, cobra.ShellCompDirectiveNoFileComp
}

type schedulePauser interface {
	SchedulePause(context.Context, string, ferricstore.ScheduleStatusOptions) (ferricstore.ScheduleRecord, error)
}

func newSchedulePauseCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "pause <id>",
		Short:   "Pause future schedule fires",
		Example: "  ferric workflow schedule pause daily-report",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "pause schedule", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				pauser, err := requireClientCapability[schedulePauser](client, "FerricFlow schedule pause")
				if err != nil {
					return nil, err
				}
				result, err := pauser.SchedulePause(ctx, args[0], ferricstore.ScheduleStatusOptions{})
				return scheduleResultOutput(&result), err
			})
		},
	}
}

type scheduleResumer interface {
	ScheduleResume(context.Context, string, ferricstore.ScheduleStatusOptions) (ferricstore.ScheduleRecord, error)
}

func newScheduleResumeCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "resume <id>",
		Short:   "Resume a paused schedule",
		Example: "  ferric workflow schedule resume daily-report",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "resume schedule", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				resumer, err := requireClientCapability[scheduleResumer](client, "FerricFlow schedule resume")
				if err != nil {
					return nil, err
				}
				result, err := resumer.ScheduleResume(ctx, args[0], ferricstore.ScheduleStatusOptions{})
				return scheduleResultOutput(&result), err
			})
		},
	}
}

type scheduleTriggerer interface {
	ScheduleFire(context.Context, string, ferricstore.ScheduleFireOptions) (ferricstore.ScheduleFireResult, error)
}

func newScheduleTriggerCommand(dependencies dependencies) *cobra.Command {
	var at string
	command := &cobra.Command{
		Use:     "trigger <id>",
		Aliases: []string{"fire"},
		Short:   "Trigger one schedule manually",
		Example: "  ferric workflow schedule trigger daily-report\n" +
			"  ferric workflow schedule fire daily-report --at 2026-08-01T08:00:00Z",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			var options ferricstore.ScheduleFireOptions
			if at != "" {
				value, err := parseAbsoluteMilliseconds("at", at)
				if err != nil {
					return err
				}
				options.FireAtMS = &value
			}
			return runNetworkCommand(command, dependencies, "trigger schedule", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				triggerer, err := requireClientCapability[scheduleTriggerer](client, "FerricFlow schedule trigger")
				if err != nil {
					return nil, err
				}
				result, err := triggerer.ScheduleFire(ctx, args[0], options)
				return scheduleFireOutput(result), err
			})
		},
	}
	command.Flags().StringVar(&at, "at", "", "logical fire time as RFC3339 or Unix milliseconds")
	return command
}

type scheduleDueFirer interface {
	ScheduleFireDue(context.Context, ferricstore.ScheduleFireDueOptions) (ferricstore.ScheduleFireDueResult, error)
}

func newScheduleFireDueCommand(dependencies dependencies) *cobra.Command {
	var worker string
	var lease time.Duration
	var wait time.Duration
	var limit int
	command := &cobra.Command{
		Use:   "fire-due",
		Short: "Claim and fire due schedules",
		Long:  "Run one bounded scheduler iteration. This is intended for scheduler automation, not normal manual triggering.",
		Example: "  ferric workflow schedule fire-due --worker scheduler-1 --limit 100\n" +
			"  ferric workflow schedule fire-due --worker scheduler-1 --wait 5s --lease 30s",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(worker) == "" {
				return errors.New("worker is required; use --worker <name>")
			}
			leaseMS, err := positiveMilliseconds("lease", lease)
			if err != nil {
				return err
			}
			if wait < 0 {
				return errors.New("wait must not be negative")
			}
			if limit <= 0 {
				return errors.New("limit must be greater than zero")
			}
			if err := validateBlockingWait(dependencies, wait, "--wait"); err != nil {
				return err
			}
			options := ferricstore.ScheduleFireDueOptions{
				Worker:  worker,
				LeaseMS: &leaseMS,
				Limit:   ferricstore.Int(limit),
			}
			if wait > 0 {
				waitMS := wait.Milliseconds()
				options.BlockMS = &waitMS
			}
			return runNetworkCommand(command, dependencies, "fire due schedules", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				firer, err := requireClientCapability[scheduleDueFirer](client, "FerricFlow due schedule runner")
				if err != nil {
					return nil, err
				}
				result, err := firer.ScheduleFireDue(ctx, options)
				return outputcontract.ScheduleFireDue(result), err
			})
		},
	}
	command.Flags().StringVar(&worker, "worker", "", "scheduler worker identity (required)")
	command.Flags().DurationVar(&lease, "lease", 30*time.Second, "scheduler claim lease")
	command.Flags().DurationVar(&wait, "wait", 0, "wait for due schedules for up to this duration")
	command.Flags().IntVar(&limit, "limit", 100, "maximum schedules to claim")
	return command
}

type scheduleDeleter interface {
	ScheduleDelete(context.Context, string, ferricstore.ScheduleStatusOptions) error
}

func newScheduleDeleteCommand(dependencies dependencies) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:     "delete <id>",
		Short:   "Permanently delete a schedule",
		Long:    "Delete schedule metadata and stop future fires. This command requires --yes and never confirms implicitly.",
		Example: "  ferric workflow schedule delete daily-report --yes",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !yes {
				return errors.New("schedule deletion requires --yes")
			}
			return runNetworkCommand(command, dependencies, "delete schedule", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				deleter, err := requireClientCapability[scheduleDeleter](client, "FerricFlow schedule deletion")
				if err != nil {
					return nil, err
				}
				err = deleter.ScheduleDelete(ctx, args[0], ferricstore.ScheduleStatusOptions{})
				return map[string]any{"id": args[0], "deleted": err == nil}, err
			})
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "confirm permanent deletion")
	return command
}

func scheduleResultOutput(result *ferricstore.ScheduleRecord) any {
	if result == nil {
		return nil
	}
	output := map[string]any{
		"id":              result.ID,
		"kind":            result.Kind,
		"state":           result.State,
		"target":          result.Target,
		"created_at_ms":   result.CreatedAtMS,
		"fire_count":      result.FireCount,
		"attempts":        result.Attempts,
		"coalesced_count": result.CoalescedCount,
		"skipped_count":   result.SkippedCount,
	}
	putOutput(output, "flow_id", result.FlowID)
	putOutput(output, "timezone", result.Timezone)
	putOutput(output, "cron", result.Cron)
	putOutput(output, "catchup_policy", result.CatchupPolicy)
	putOutput(output, "overlap_policy", result.OverlapPolicy)
	putOutput(output, "last_overlap_target_id", result.LastOverlapTargetID)
	putOutput(output, "last_overlap_reason", result.LastOverlapReason)
	putOutput(output, "end_reason", result.EndReason)
	putOutput(output, "last_planning_error", result.LastPlanningError)
	putOutput(output, "last_target_id", result.LastTargetID)
	putOptionalInt64(output, "every_ms", result.EveryMS)
	putOptionalInt64(output, "overlap_retry_ms", result.OverlapRetryMS)
	putOptionalInt64(output, "next_run_at_ms", result.NextRunAtMS)
	putOptionalInt64(output, "last_fire_at_ms", result.LastFireAtMS)
	putOptionalInt64(output, "max_fires", result.MaxFires)
	putOptionalInt64(output, "end_at_ms", result.EndAtMS)
	putOptionalInt64(output, "last_catchup_at_ms", result.LastCatchupAtMS)
	if result.LastCatchupAtMS != nil {
		output["last_coalesced_count"] = result.LastCoalescedCount
	}
	putOptionalInt64(output, "last_overlap_at_ms", result.LastOverlapAtMS)
	putOptionalInt64(output, "last_skipped_at_ms", result.LastSkippedAtMS)
	putOptionalInt64(output, "overlap_queued_due_at_ms", result.OverlapQueuedDueAtMS)
	return output
}

func putOptionalInt64(output map[string]any, name string, value *int64) {
	if value != nil {
		output[name] = *value
	}
}

func scheduleResultsOutput(results []ferricstore.ScheduleRecord) any {
	output := make([]any, len(results))
	for index := range results {
		output[index] = scheduleResultOutput(&results[index])
	}
	return output
}

func scheduleFireOutput(result ferricstore.ScheduleFireResult) any {
	return map[string]any{
		"fired":     result.Fired,
		"skipped":   result.Skipped,
		"target_id": result.TargetID,
		"reason":    result.Reason,
		"schedule":  scheduleResultOutput(&result.Schedule),
	}
}
