package cli

import (
	"context"
	"errors"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	ferricstore "github.com/ferricstore/ferricstore-go"
	"github.com/spf13/cobra"
)

type flowExistenceReader interface {
	Exists(context.Context, string, ferricstore.ReadOptions) (bool, error)
}

func newFlowExistsCommand(dependencies dependencies) *cobra.Command {
	var flags flowReadFlags
	command := &cobra.Command{
		Use:     "exists <type>",
		Short:   "Check whether any matching workflow execution exists",
		Example: "  ferric workflow exists order --partition tenant-a --state running",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			options, err := flags.options(command)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "check workflow existence", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[flowExistenceReader](client, "FerricFlow existence checks")
				if err != nil {
					return nil, err
				}
				return reader.Exists(ctx, args[0], options)
			})
		},
	}
	flags.addAdmin(command, true)
	return command
}

type flowStateCounter interface {
	CountByState(context.Context, string, string, ferricstore.ReadOptions) (int64, error)
}

func newFlowCountByStateCommand(dependencies dependencies) *cobra.Command {
	var flags flowReadFlags
	command := &cobra.Command{
		Use:     "count-by-state <type> <state>",
		Short:   "Count executions in one workflow state",
		Example: "  ferric workflow count-by-state order failed --partition tenant-a",
		Args:    cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			options, err := flags.options(command)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "count workflows by state", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				counter, err := requireClientCapability[flowStateCounter](client, "FerricFlow state counts")
				if err != nil {
					return nil, err
				}
				count, err := counter.CountByState(ctx, args[0], args[1], options)
				return map[string]any{"type": args[0], "state": args[1], "count": count}, err
			})
		},
	}
	flags.addAdmin(command, false)
	return command
}

type flowAttributeReader interface {
	Attributes(context.Context, string, ferricstore.ReadOptions) ([]map[string]any, error)
}

func newFlowAttributesCommand(dependencies dependencies) *cobra.Command {
	var flags flowReadFlags
	command := &cobra.Command{
		Use:     "attributes <type>",
		Short:   "List indexed attributes for a workflow type",
		Example: "  ferric workflow attributes order --partition tenant-a --limit 25",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			options, err := flags.options(command)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "list workflow attributes", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[flowAttributeReader](client, "FerricFlow attribute indexes")
				if err != nil {
					return nil, err
				}
				return reader.Attributes(ctx, args[0], options)
			})
		},
	}
	flags.addAdmin(command, true)
	return command
}

type flowAttributeValueReader interface {
	AttributeValues(context.Context, string, string, ferricstore.ReadOptions) ([]map[string]any, error)
}

func newFlowAttributeValuesCommand(dependencies dependencies) *cobra.Command {
	var flags flowReadFlags
	command := &cobra.Command{
		Use:     "attribute-values <type> <attribute>",
		Short:   "List indexed values for one workflow attribute",
		Example: "  ferric workflow attribute-values order region --partition tenant-a",
		Args:    cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			options, err := flags.options(command)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "list workflow attribute values", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[flowAttributeValueReader](client, "FerricFlow attribute values")
				if err != nil {
					return nil, err
				}
				return reader.AttributeValues(ctx, args[0], args[1], options)
			})
		},
	}
	flags.addAdmin(command, true)
	return command
}

type governanceLedgerReader interface {
	GovernanceLedger(context.Context, string, ferricstore.GovernanceLedgerOptions) ([]map[string]any, error)
}

func newGovernanceLedgerCommand(dependencies dependencies) *cobra.Command {
	var (
		partition string
		limit     int
		fromMS    int64
		toMS      int64
		reverse   bool
	)
	command := &cobra.Command{
		Use:   "ledger <id>",
		Short: "List governance ledger events for a workflow",
		Example: "  ferric workflow governance ledger order-42 --partition tenant-a\n" +
			"  ferric workflow governance ledger order-42 --from-ms 1700000000000 --reverse",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			options := ferricstore.GovernanceLedgerOptions{PartitionKey: partition}
			if command.Flags().Changed("limit") {
				if limit <= 0 {
					return errors.New("limit must be greater than zero")
				}
				options.Limit = ferricstore.Int(limit)
			}
			if command.Flags().Changed("from-ms") {
				if fromMS < 0 {
					return errors.New("from-ms must not be negative")
				}
				options.FromMS = ferricstore.Int64(fromMS)
			}
			if command.Flags().Changed("to-ms") {
				if toMS < 0 {
					return errors.New("to-ms must not be negative")
				}
				options.ToMS = ferricstore.Int64(toMS)
			}
			if options.FromMS != nil && options.ToMS != nil && *options.FromMS > *options.ToMS {
				return errors.New("from-ms cannot exceed to-ms")
			}
			if command.Flags().Changed("reverse") {
				options.Rev = ferricstore.Bool(reverse)
			}
			return runNetworkCommand(command, dependencies, "read governance ledger", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[governanceLedgerReader](client, "governance ledger")
				if err != nil {
					return nil, err
				}
				return reader.GovernanceLedger(ctx, args[0], options)
			})
		},
	}
	command.Flags().StringVar(&partition, "partition", "", "partition key")
	command.Flags().IntVar(&limit, "limit", 0, "maximum ledger events to return")
	command.Flags().Int64Var(&fromMS, "from-ms", 0, "minimum event timestamp in Unix milliseconds")
	command.Flags().Int64Var(&toMS, "to-ms", 0, "maximum event timestamp in Unix milliseconds")
	command.Flags().BoolVar(&reverse, "reverse", false, "return newest events first")
	return command
}

type flowRetentionCleaner interface {
	RetentionCleanup(context.Context, ferricstore.RetentionCleanupOptions) (map[string]any, error)
}

func newFlowRetentionCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "retention",
		Short: "Operate workflow retention maintenance",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(newFlowRetentionCleanupCommand(dependencies))
	return command
}

func newFlowRetentionCleanupCommand(dependencies dependencies) *cobra.Command {
	var (
		limit int
		nowMS int64
		yes   bool
	)
	command := &cobra.Command{
		Use:     "cleanup",
		Short:   "Delete expired retained workflow records",
		Example: "  ferric workflow retention cleanup --limit 100 --yes",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := requireProtocolConfirmation(yes, "retention cleanup", []any{"FLOW.RETENTION_CLEANUP"}); err != nil {
				return err
			}
			options := ferricstore.RetentionCleanupOptions{}
			if command.Flags().Changed("limit") {
				if limit <= 0 {
					return errors.New("limit must be greater than zero")
				}
				options.Limit = ferricstore.Int(limit)
			}
			if command.Flags().Changed("now-ms") {
				if nowMS < 0 {
					return errors.New("now-ms must not be negative")
				}
				options.NowMS = ferricstore.Int64(nowMS)
			}
			return runNetworkCommand(command, dependencies, "clean up workflow retention", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				cleaner, err := requireClientCapability[flowRetentionCleaner](client, "FerricFlow retention cleanup")
				if err != nil {
					return nil, err
				}
				return cleaner.RetentionCleanup(ctx, options)
			})
		},
	}
	command.Flags().IntVar(&limit, "limit", 0, "maximum records to delete")
	command.Flags().Int64Var(&nowMS, "now-ms", 0, "cleanup clock in Unix milliseconds")
	command.Flags().BoolVar(&yes, "yes", false, "confirm deletion of expired retained records")
	return command
}
