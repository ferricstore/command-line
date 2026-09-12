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

type flowNamedValueWriter interface {
	PutValue(context.Context, string, any, ferricstore.ValuePutOptions) (any, error)
}

func newWorkflowNamedValuePutCommand(dependencies dependencies) *cobra.Command {
	var (
		partition string
		owner     string
		override  bool
		input     valueInput
	)
	command := &cobra.Command{
		Use:   "put <name> [value]",
		Short: "Publish a named value owned by a workflow",
		Long:  "Store or replace a named content-addressed value. Named values inherit their owner workflow's retention.",
		Example: "  ferric workflow values put receipt '{\"status\":\"paid\"}' --json --owner order-42\n" +
			"  ferric workflow values put document --file document.json --json --owner order-42 --override",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(owner) == "" {
				return errors.New("owner is required for a named value; use --owner <flow-id>")
			}
			var positional *string
			if len(args) == 2 {
				positional = &args[1]
			}
			value, present, err := input.read(command, positional)
			if err != nil {
				return err
			}
			if !present {
				return errors.New("value is required as an argument or --file")
			}
			options := ferricstore.ValuePutOptions{
				PartitionKey: partition,
				OwnerFlowID:  owner,
			}
			if command.Flags().Changed("override") {
				options.Override = ferricstore.Bool(override)
			}
			return runNetworkCommand(command, dependencies, "put named workflow value", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				writer, err := requireClientCapability[flowNamedValueWriter](client, "FerricFlow named value writes")
				if err != nil {
					return nil, err
				}
				return writer.PutValue(ctx, args[0], value, options)
			})
		},
	}
	command.Flags().StringVar(&partition, "partition", "", "owner workflow partition key")
	command.Flags().StringVar(&owner, "owner", "", "owner workflow ID (required)")
	command.Flags().BoolVar(&override, "override", false, "replace an existing named value")
	input.addFlags(command, "value")
	return command
}

type flowValueUploader interface {
	ValuePut(context.Context, any, ferricstore.ValuePutOptions) (any, error)
}

func newWorkflowValueUploadCommand(dependencies dependencies) *cobra.Command {
	var (
		partition string
		owner     string
		ttl       time.Duration
		input     valueInput
	)
	command := &cobra.Command{
		Use:   "upload [value]",
		Short: "Upload an unnamed content-addressed value",
		Example: "  ferric workflow values upload artifact --ttl 24h\n" +
			"  ferric workflow values upload --file artifact.json --json --partition tenant-a",
		Args: cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			var positional *string
			if len(args) == 1 {
				positional = &args[0]
			}
			value, present, err := input.read(command, positional)
			if err != nil {
				return err
			}
			if !present {
				return errors.New("value is required as an argument or --file")
			}
			options := ferricstore.ValuePutOptions{PartitionKey: partition, OwnerFlowID: owner}
			if command.Flags().Changed("ttl") {
				value, err := positiveMilliseconds("ttl", ttl)
				if err != nil {
					return err
				}
				options.TTLMS = &value
			}
			return runNetworkCommand(command, dependencies, "upload workflow value", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				writer, err := requireClientCapability[flowValueUploader](client, "FerricFlow value uploads")
				if err != nil {
					return nil, err
				}
				return writer.ValuePut(ctx, value, options)
			})
		},
	}
	command.Flags().StringVar(&partition, "partition", "", "value partition key")
	command.Flags().StringVar(&owner, "owner", "", "optional owner workflow ID")
	command.Flags().DurationVar(&ttl, "ttl", 0, "retention time for an unnamed value")
	input.addFlags(command, "value")
	return command
}
