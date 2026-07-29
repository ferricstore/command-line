package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ferricstore/command-line/internal/commandsafety"
	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	ferricstore "github.com/ferricstore/ferricstore-go"
	"github.com/spf13/cobra"
)

const maxTransactionInputBytes = 16 * 1024 * 1024

type transactionStarter interface {
	Watch(context.Context, ...string) error
	Transaction(context.Context) (*ferricstore.Transaction, error)
	TransactionForKeys(context.Context, ...string) (*ferricstore.Transaction, error)
}

func newStoreTransactionCommand(dependencies dependencies) *cobra.Command {
	var keys []string
	var watchedKeys []string
	var yes bool
	command := &cobra.Command{
		Use:   "transaction <file>",
		Short: "Execute a JSON batch atomically",
		Long: "Execute a JSON array of command arrays in one connection-affine MULTI/EXEC transaction. " +
			"Use - to read stdin. In cluster mode, provide every routing key with --key; all keys must share a slot. " +
			"Use --watch for an optimistic transaction that aborts if watched keys change.",
		Example: "  ferric store transaction commands.json\n" +
			"  printf '%s' '[[\"SET\",\"counter\",\"1\"],[\"INCR\",\"counter\"]]' | ferric store transaction - --key counter\n" +
			"  ferric store transaction admin-commands.json --yes",
		GroupID: storeEscapeHatchGroup,
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if len(keys) != 0 && len(watchedKeys) != 0 {
				return errors.New("use --key or --watch, not both")
			}
			commands, err := readTransactionCommands(command, args[0])
			if err != nil {
				return err
			}
			if index, sensitive := commandsafety.FirstRequiringConfirmation(commands); sensitive && !yes {
				return fmt.Errorf("transaction command %d requires --yes", index+1)
			}
			return runNetworkCommand(command, dependencies, "execute transaction", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				starter, err := requireClientCapability[transactionStarter](client, "SDK transactions")
				if err != nil {
					return nil, err
				}
				var transaction *ferricstore.Transaction
				switch {
				case len(watchedKeys) != 0:
					if err := starter.Watch(ctx, watchedKeys...); err != nil {
						return nil, err
					}
					transaction, err = starter.Transaction(ctx)
				case len(keys) == 0:
					transaction, err = starter.Transaction(ctx)
				default:
					transaction, err = starter.TransactionForKeys(ctx, keys...)
				}
				if err != nil {
					return nil, err
				}
				for index, item := range commands {
					if _, err := transaction.Command(ctx, item...); err != nil {
						_ = transaction.Discard(ctx)
						return nil, fmt.Errorf("queue transaction command %d: %w", index+1, err)
					}
				}
				return transaction.Exec(ctx)
			})
		},
	}
	command.Flags().StringSliceVar(&keys, "key", nil, "routing key for cluster slot selection; repeatable")
	command.Flags().StringSliceVar(&watchedKeys, "watch", nil, "abort if this key changes before EXEC; repeatable")
	command.Flags().BoolVar(&yes, "yes", false, "confirm safety-sensitive commands in the transaction")
	return command
}

func readTransactionCommands(command *cobra.Command, path string) ([][]any, error) {
	var input io.Reader
	if path == "-" {
		input = command.InOrStdin()
	} else {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open transaction file: %w", err)
		}
		defer func() { _ = file.Close() }()
		input = file
	}
	limited := &io.LimitedReader{R: input, N: maxTransactionInputBytes + 1}
	decoder := json.NewDecoder(limited)
	decoder.UseNumber()
	var commands [][]any
	if err := decoder.Decode(&commands); err != nil {
		return nil, fmt.Errorf("parse transaction JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("transaction input contains trailing JSON")
		}
		return nil, fmt.Errorf("parse trailing transaction input: %w", err)
	}
	if limited.N == 0 {
		return nil, errors.New("transaction input exceeds 16 MiB")
	}
	if len(commands) == 0 {
		return nil, errors.New("transaction must contain at least one command")
	}
	for index, item := range commands {
		if len(item) == 0 {
			return nil, fmt.Errorf("transaction command %d is empty", index+1)
		}
		name, ok := item[0].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("transaction command %d must start with a command name", index+1)
		}
		for argumentIndex := range item {
			item[argumentIndex] = normalizeJSONNumber(item[argumentIndex])
		}
	}
	return commands, nil
}

func normalizeJSONNumber(value any) any {
	number, ok := value.(json.Number)
	if !ok {
		return value
	}
	if integer, err := number.Int64(); err == nil {
		return integer
	}
	if floating, err := number.Float64(); err == nil {
		return floating
	}
	return number.String()
}
