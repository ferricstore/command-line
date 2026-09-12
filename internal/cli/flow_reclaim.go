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

type flowReclaimFlags struct {
	worker            string
	partition         string
	lease             time.Duration
	limit             int
	payload           bool
	includeAttributes bool
	values            []string
}

func (flags *flowReclaimFlags) add(command *cobra.Command, supportsValues bool) {
	command.Flags().StringVar(&flags.worker, "worker", "", "worker taking ownership of expired leases (required)")
	command.Flags().StringVar(&flags.partition, "partition", "", "partition key")
	command.Flags().DurationVar(&flags.lease, "lease", 30*time.Second, "new lease duration")
	command.Flags().IntVar(&flags.limit, "limit", 1, "maximum expired leases to reclaim")
	command.Flags().BoolVar(&flags.payload, "payload", true, "include payloads")
	command.Flags().BoolVar(&flags.includeAttributes, "attributes", true, "include attributes")
	if supportsValues {
		command.Flags().StringArrayVar(&flags.values, "value", nil, "named value to include; repeatable")
	}
}

func (flags flowReclaimFlags) options(flowType string, jobOnly bool) (ferricstore.ReclaimOptions, error) {
	if strings.TrimSpace(flags.worker) == "" {
		return ferricstore.ReclaimOptions{}, errors.New("worker is required; use --worker <name>")
	}
	leaseMS, err := positiveMilliseconds("lease", flags.lease)
	if err != nil {
		return ferricstore.ReclaimOptions{}, err
	}
	if flags.limit <= 0 {
		return ferricstore.ReclaimOptions{}, errors.New("limit must be greater than zero")
	}
	return ferricstore.ReclaimOptions{
		Type:              flowType,
		State:             "running",
		Worker:            flags.worker,
		PartitionKey:      flags.partition,
		LeaseMS:           leaseMS,
		Limit:             flags.limit,
		JobOnly:           jobOnly,
		Payload:           ferricstore.Bool(flags.payload),
		Values:            flags.values,
		IncludeAttributes: ferricstore.Bool(flags.includeAttributes),
	}, nil
}

type flowJobReclaimer interface {
	ReclaimJobs(context.Context, ferricstore.ReclaimOptions) ([]ferricstore.ClaimedItem, error)
}

func newQueueReclaimCommand(dependencies dependencies) *cobra.Command {
	var flags flowReclaimFlags
	command := &cobra.Command{
		Use:   "reclaim <type>",
		Short: "Take ownership of jobs whose leases expired",
		Long:  "Reclaim expired running jobs and issue fresh lease and fencing tokens to a worker.",
		Example: "  ferric queue reclaim email --worker mailer-2\n" +
			"  ferric queue reclaim email --worker mailer-2 --lease 1m --limit 10 --partition tenant-a",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			options, err := flags.options(args[0], true)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "reclaim jobs", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reclaimer, err := requireClientCapability[flowJobReclaimer](client, "FerricFlow job reclaim")
				if err != nil {
					return nil, err
				}
				claims, err := reclaimer.ReclaimJobs(ctx, options)
				return flowClaimsOutput(claims), err
			})
		},
	}
	flags.add(command, false)
	return command
}

type flowReclaimer interface {
	Reclaim(context.Context, ferricstore.ReclaimOptions) ([]ferricstore.FlowRecord, error)
}

func newWorkflowReclaimCommand(dependencies dependencies) *cobra.Command {
	var flags flowReclaimFlags
	command := &cobra.Command{
		Use:   "reclaim <type>",
		Short: "Take ownership of workflows whose leases expired",
		Long:  "Reclaim expired running workflows and issue fresh lease and fencing tokens to a worker.",
		Example: "  ferric workflow reclaim order --worker worker-2\n" +
			"  ferric workflow reclaim order --worker worker-2 --value receipt --payload=false",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			options, err := flags.options(args[0], false)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "reclaim workflows", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reclaimer, err := requireClientCapability[flowReclaimer](client, "FerricFlow workflow reclaim")
				if err != nil {
					return nil, err
				}
				records, err := reclaimer.Reclaim(ctx, options)
				return flowRecordsOutput(records), err
			})
		},
	}
	flags.add(command, true)
	return command
}
