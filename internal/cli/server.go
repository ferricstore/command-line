package cli

import (
	"context"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	"github.com/spf13/cobra"
)

type pingClient interface {
	Ping(context.Context, ...string) (string, error)
}

func newServerCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "server",
		Short:   "Inspect and manage a FerricStore server",
		Example: "  ferric server ping\n  ferric server info\n  ferric server metrics",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newServerPingCommand(dependencies),
		newSDKCommand(dependencies, sdkCommandSpec{
			name: "echo", use: "echo <message>", short: "Return a message through the server",
			example: "  ferric server echo hello",
			wire:    []any{"ECHO"}, minArgs: 1, maxArgs: 1,
		}),
		newSDKCommand(dependencies, sdkCommandSpec{
			name: "info", use: "info [section]", short: "Show server information",
			example: "  ferric server info\n  ferric server info persistence",
			wire:    []any{"INFO"}, minArgs: 0, maxArgs: 1,
		}),
		newSDKCommand(dependencies, sdkCommandSpec{
			name: "capabilities", use: "capabilities", short: "List commands supported by the server",
			example: "  ferric server capabilities",
			wire:    []any{"COMMAND"}, minArgs: 0, maxArgs: 0,
		}),
		newSDKCommand(dependencies, sdkCommandSpec{
			name: "metrics", use: "metrics", short: "Show FerricStore metrics",
			example: "  ferric server metrics --output json",
			wire:    []any{"FERRICSTORE.METRICS"}, minArgs: 0, maxArgs: 0,
		}),
		newSDKCommand(dependencies, sdkCommandSpec{
			name: "health", use: "health", short: "Show cluster health",
			example: "  ferric server health",
			wire:    []any{"CLUSTER.HEALTH"}, minArgs: 0, maxArgs: 0,
		}),
		newSDKCommand(dependencies, sdkCommandSpec{
			name: "stats", use: "stats", short: "Show cluster statistics",
			example: "  ferric server stats",
			wire:    []any{"CLUSTER.STATS"}, minArgs: 0, maxArgs: 0,
		}),
		newSDKCommand(dependencies, sdkCommandSpec{
			name: "key-info", use: "key-info <key>", short: "Inspect FerricStore metadata for a key",
			example: "  ferric server key-info user:42",
			wire:    []any{"FERRICSTORE.KEY_INFO"}, minArgs: 1, maxArgs: 1,
		}),
		newServerDoctorCommand(dependencies),
		newServerConfigCommand(dependencies),
		newServerSlowlogCommand(dependencies),
		newServerClientCommand(dependencies),
		newServerPersistenceCommand(dependencies),
		newSDKCommand(dependencies, sdkCommandSpec{
			name: "memory-usage", use: "memory-usage <key>", short: "Estimate memory used by a key",
			example: "  ferric server memory-usage user:42",
			wire:    []any{"MEMORY", "USAGE"}, minArgs: 1, maxArgs: 1,
		}),
		newConfirmedSDKCommand(dependencies, sdkCommandSpec{
			name: "flush", aliases: []string{"flush-db"}, use: "flush [ASYNC|SYNC]", short: "Delete every key in the database",
			long:    "Delete every key in the selected database. This command requires --yes.",
			example: "  ferric server flush --yes\n  ferric server flush --yes ASYNC",
			wire:    []any{"FLUSHDB"}, minArgs: 0, maxArgs: 1,
		}),
	)
	return command
}

func newServerConfigCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "config",
		Short:   "Read and change server configuration",
		Example: "  ferric server config get 'persistence.*'\n  ferric server config set --yes persistence.sync interval",
		Args:    cobra.NoArgs,
		RunE:    func(command *cobra.Command, _ []string) error { return command.Help() },
	}
	for _, spec := range []sdkCommandSpec{
		{name: "get", use: "get <pattern>", short: "Read replicated configuration", example: "  ferric server config get 'persistence.*'", wire: []any{"CONFIG", "GET"}, minArgs: 1, maxArgs: 1},
		{name: "get-local", use: "get-local <key>", short: "Read node-local configuration", example: "  ferric server config get-local cache.size", wire: []any{"CONFIG", "GET", "LOCAL"}, minArgs: 1, maxArgs: 1},
	} {
		command.AddCommand(newSDKCommand(dependencies, spec))
	}
	for _, spec := range []sdkCommandSpec{
		{name: "set", use: "set <key> <value>", short: "Change replicated configuration", long: "Change replicated configuration. This command requires --yes.", example: "  ferric server config set --yes persistence.sync interval", wire: []any{"CONFIG", "SET"}, minArgs: 2, maxArgs: 2},
		{name: "set-local", use: "set-local <key> <value>", short: "Change node-local configuration", long: "Change node-local configuration. This command requires --yes.", example: "  ferric server config set-local --yes cache.size 10000", wire: []any{"CONFIG", "SET", "LOCAL"}, minArgs: 2, maxArgs: 2},
		{name: "reset-stats", use: "reset-stats", short: "Reset server statistics and slowlog", long: "Reset server statistics and the slowlog. This command requires --yes.", example: "  ferric server config reset-stats --yes", wire: []any{"CONFIG", "RESETSTAT"}, minArgs: 0, maxArgs: 0},
		{name: "rewrite", use: "rewrite", short: "Persist the current configuration", long: "Persist configuration changes. This command requires --yes.", example: "  ferric server config rewrite --yes", wire: []any{"CONFIG", "REWRITE"}, minArgs: 0, maxArgs: 0},
	} {
		command.AddCommand(newConfirmedSDKCommand(dependencies, spec))
	}
	return command
}

func newServerSlowlogCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "slowlog",
		Short:   "Inspect the slow command log",
		Example: "  ferric server slowlog get 20\n  ferric server slowlog length",
		Args:    cobra.NoArgs,
		RunE:    func(command *cobra.Command, _ []string) error { return command.Help() },
	}
	command.AddCommand(
		newSDKCommand(dependencies, sdkCommandSpec{name: "get", use: "get [count]", short: "Get recent slow commands", example: "  ferric server slowlog get 20", wire: []any{"SLOWLOG", "GET"}, minArgs: 0, maxArgs: 1}),
		newSDKCommand(dependencies, sdkCommandSpec{name: "length", aliases: []string{"len"}, use: "length", short: "Count slowlog entries", example: "  ferric server slowlog length", wire: []any{"SLOWLOG", "LEN"}, minArgs: 0, maxArgs: 0}),
		newConfirmedSDKCommand(dependencies, sdkCommandSpec{name: "reset", use: "reset", short: "Clear the slowlog", long: "Clear the slowlog. This command requires --yes.", example: "  ferric server slowlog reset --yes", wire: []any{"SLOWLOG", "RESET"}, minArgs: 0, maxArgs: 0}),
	)
	return command
}

func newServerClientCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "client",
		Short:   "Inspect connected clients",
		Example: "  ferric server client id\n  ferric server client list",
		Args:    cobra.NoArgs,
		RunE:    func(command *cobra.Command, _ []string) error { return command.Help() },
	}
	for _, spec := range []sdkCommandSpec{
		{name: "id", use: "id", short: "Show this connection's server ID", example: "  ferric server client id", wire: []any{"CLIENT", "ID"}, minArgs: 0, maxArgs: 0},
		{name: "info", use: "info", short: "Show this connection's information", example: "  ferric server client info", wire: []any{"CLIENT", "INFO"}, minArgs: 0, maxArgs: 0},
		{name: "list", use: "list [TYPE <type>]", short: "List connected clients", example: "  ferric server client list\n  ferric server client list TYPE normal", wire: []any{"CLIENT", "LIST"}, minArgs: 0, maxArgs: 2},
		{name: "tracking-info", use: "tracking-info", short: "Show client tracking configuration", example: "  ferric server client tracking-info", wire: []any{"CLIENT", "TRACKINGINFO"}, minArgs: 0, maxArgs: 0},
		{name: "redirect", use: "redirect", short: "Show the client tracking redirect ID", example: "  ferric server client redirect", wire: []any{"CLIENT", "GETREDIR"}, minArgs: 0, maxArgs: 0},
	} {
		command.AddCommand(newSDKCommand(dependencies, spec))
	}
	return command
}

func newServerPersistenceCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "persistence",
		Short:   "Inspect or request persistence checkpoints",
		Example: "  ferric server persistence last-save\n  ferric server persistence save",
		Args:    cobra.NoArgs,
		RunE:    func(command *cobra.Command, _ []string) error { return command.Help() },
	}
	for _, spec := range []sdkCommandSpec{
		{name: "save", use: "save", short: "Request a synchronous persistence checkpoint", example: "  ferric server persistence save", wire: []any{"SAVE"}, minArgs: 0, maxArgs: 0},
		{name: "background-save", aliases: []string{"bgsave"}, use: "background-save", short: "Request a background persistence checkpoint", example: "  ferric server persistence background-save", wire: []any{"BGSAVE"}, minArgs: 0, maxArgs: 0},
		{name: "last-save", use: "last-save", short: "Show the last persistence timestamp", example: "  ferric server persistence last-save", wire: []any{"LASTSAVE"}, minArgs: 0, maxArgs: 0},
	} {
		command.AddCommand(newSDKCommand(dependencies, spec))
	}
	return command
}

func newServerDoctorCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "doctor",
		Short:   "Run and inspect bounded server diagnostics",
		Example: "  ferric server doctor check\n  ferric server doctor list",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	for _, spec := range []sdkCommandSpec{
		{
			name: "check", use: "check [SCOPE <scope>]", short: "Run a bounded diagnostic check",
			example: "  ferric server doctor check\n  ferric server doctor check SCOPE FLOW_LMDB",
			wire:    []any{"FERRICSTORE.DOCTOR", "CHECK"}, minArgs: 0, maxArgs: -1,
		},
		{
			name: "start-check", use: "start-check [SCOPE <scope>]", short: "Start a diagnostic check in the background",
			example: "  ferric server doctor start-check\n  ferric server doctor start-check SCOPE BITCASK",
			wire:    []any{"FERRICSTORE.DOCTOR", "START", "CHECK"}, minArgs: 0, maxArgs: -1,
		},
		{
			name: "status", use: "status <job-id>", short: "Show a background diagnostic job",
			example: "  ferric server doctor status doctor-1-123",
			wire:    []any{"FERRICSTORE.DOCTOR", "STATUS"}, minArgs: 1, maxArgs: 1,
		},
		{
			name: "list", use: "list", short: "List background diagnostic jobs",
			example: "  ferric server doctor list",
			wire:    []any{"FERRICSTORE.DOCTOR", "LIST"}, minArgs: 0, maxArgs: 0,
		},
		{
			name: "cancel", use: "cancel <job-id>", short: "Cancel a running diagnostic job",
			example: "  ferric server doctor cancel doctor-1-123",
			wire:    []any{"FERRICSTORE.DOCTOR", "CANCEL"}, minArgs: 1, maxArgs: 1,
		},
	} {
		command.AddCommand(newSDKCommand(dependencies, spec))
	}
	command.AddCommand(newConfirmedSDKCommand(dependencies, sdkCommandSpec{
		name: "repair-projections", use: "repair-projections <scope>", short: "Start a background projection repair",
		long:    "Start a background projection reconciliation job. This command requires --yes.",
		example: "  ferric server doctor repair-projections --yes FLOW_LMDB",
		wire:    []any{"FERRICSTORE.DOCTOR", "START", "REPAIR", "PROJECTIONS", "SCOPE"}, minArgs: 1, maxArgs: 1,
	}))
	return command
}

func newServerPingCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "ping [message]",
		Short: "Check connectivity using a saved profile",
		Example: "  ferric server ping\n" +
			"  ferric server ping hello",
		Args: cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "ping", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				pinger, err := requireClientCapability[pingClient](client, "PING")
				if err != nil {
					return nil, err
				}
				return pinger.Ping(ctx, args...)
			})
		},
	}
	return command
}
