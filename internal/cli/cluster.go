package cli

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	"github.com/spf13/cobra"
)

func newClusterCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "cluster",
		Short: "Inspect and operate a FerricStore cluster",
		Long:  "Inspect topology and perform explicit cluster membership or role changes. Mutating operations require --yes.",
		Example: "  ferric cluster health\n" +
			"  ferric cluster status\n" +
			"  ferric cluster keyslot user:42",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newClusterHealthCommand(dependencies),
		newClusterStatsCommand(dependencies),
		newClusterStatusCommand(dependencies),
		newClusterRoleCommand(dependencies),
		newClusterKeySlotCommand(dependencies),
		newClusterSlotsCommand(dependencies),
		newClusterJoinCommand(dependencies),
		newClusterLeaveCommand(dependencies),
		newClusterFailoverCommand(dependencies),
		newClusterPromoteCommand(dependencies),
		newClusterDemoteCommand(dependencies),
	)
	return command
}

type clusterHealthReader interface {
	ClusterHealth(context.Context) (map[string]any, error)
}

func newClusterHealthCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "health",
		Short:   "Show cluster health",
		Example: "  ferric cluster health",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runNetworkCommand(command, dependencies, "cluster health", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[clusterHealthReader](client, "cluster health")
				if err != nil {
					return nil, err
				}
				return reader.ClusterHealth(ctx)
			})
		},
	}
}

type clusterStatsReader interface {
	ClusterStats(context.Context) (map[string]any, error)
}

func newClusterStatsCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "stats",
		Short:   "Show cluster statistics",
		Example: "  ferric cluster stats",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runNetworkCommand(command, dependencies, "cluster statistics", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[clusterStatsReader](client, "cluster statistics")
				if err != nil {
					return nil, err
				}
				return reader.ClusterStats(ctx)
			})
		},
	}
}

type clusterStatusReader interface {
	ClusterStatus(context.Context) (map[string]any, error)
}

func newClusterStatusCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Short:   "Show topology and membership status",
		Example: "  ferric cluster status",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runNetworkCommand(command, dependencies, "cluster status", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[clusterStatusReader](client, "cluster status")
				if err != nil {
					return nil, err
				}
				return reader.ClusterStatus(ctx)
			})
		},
	}
}

type clusterRoleReader interface {
	ClusterRole(context.Context) (any, error)
}

func newClusterRoleCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "role",
		Short:   "Show the connected node's cluster role",
		Example: "  ferric cluster role",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runNetworkCommand(command, dependencies, "cluster role", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[clusterRoleReader](client, "cluster role")
				if err != nil {
					return nil, err
				}
				return reader.ClusterRole(ctx)
			})
		},
	}
}

type clusterKeySlotReader interface {
	ClusterKeySlot(context.Context, string) (int64, error)
}

func newClusterKeySlotCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "keyslot <key>",
		Short:   "Return the hash slot for a key",
		Example: "  ferric cluster keyslot user:42",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "cluster key slot", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[clusterKeySlotReader](client, "cluster key slots")
				if err != nil {
					return nil, err
				}
				return reader.ClusterKeySlot(ctx, args[0])
			})
		},
	}
}

type clusterSlotsReader interface {
	ClusterSlots(context.Context) (any, error)
}

func newClusterSlotsCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "slots",
		Short:   "Show cluster slot ownership",
		Example: "  ferric cluster slots",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runNetworkCommand(command, dependencies, "cluster slots", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[clusterSlotsReader](client, "cluster slots")
				if err != nil {
					return nil, err
				}
				return reader.ClusterSlots(ctx)
			})
		},
	}
}

type clusterJoiner interface {
	ClusterJoin(context.Context, string, bool) (bool, error)
}

func newClusterJoinCommand(dependencies dependencies) *cobra.Command {
	var replace bool
	return newConfirmedClusterCommand(dependencies, confirmedClusterSpec{
		use: "join <node>", short: "Join a node to the cluster",
		example:   "  ferric cluster join 10.0.0.12:6388 --yes\n  ferric cluster join 10.0.0.12:6388 --replace --yes",
		operation: "join cluster node", args: cobra.ExactArgs(1),
		addFlags: func(command *cobra.Command) {
			command.Flags().BoolVar(&replace, "replace", false, "replace an existing member with the same identity")
		},
		run: func(ctx context.Context, client connection.Client, args []string) (any, error) {
			operator, err := requireClientCapability[clusterJoiner](client, "cluster join")
			if err != nil {
				return nil, err
			}
			return operator.ClusterJoin(ctx, args[0], replace)
		},
	})
}

type clusterLeaver interface {
	ClusterLeave(context.Context) (bool, error)
}

func newClusterLeaveCommand(dependencies dependencies) *cobra.Command {
	return newConfirmedClusterCommand(dependencies, confirmedClusterSpec{
		use: "leave", short: "Remove the connected node from the cluster",
		example: "  ferric cluster leave --yes", operation: "leave cluster", args: cobra.NoArgs,
		run: func(ctx context.Context, client connection.Client, _ []string) (any, error) {
			operator, err := requireClientCapability[clusterLeaver](client, "cluster leave")
			if err != nil {
				return nil, err
			}
			return operator.ClusterLeave(ctx)
		},
	})
}

type clusterFailoverOperator interface {
	ClusterFailover(context.Context, int, string) (bool, error)
}

func newClusterFailoverCommand(dependencies dependencies) *cobra.Command {
	return newConfirmedClusterCommand(dependencies, confirmedClusterSpec{
		use: "failover <shard-index> <target-node>", short: "Move shard leadership to another node",
		example: "  ferric cluster failover 2 node-b --yes", operation: "cluster failover", args: cobra.ExactArgs(2),
		run: func(ctx context.Context, client connection.Client, args []string) (any, error) {
			shard, err := strconv.Atoi(args[0])
			if err != nil || shard < 0 {
				return nil, errors.New("shard-index must be a non-negative integer")
			}
			operator, err := requireClientCapability[clusterFailoverOperator](client, "cluster failover")
			if err != nil {
				return nil, err
			}
			return operator.ClusterFailover(ctx, shard, args[1])
		},
	})
}

type clusterPromoter interface {
	ClusterPromote(context.Context, string) (bool, error)
}

func newClusterPromoteCommand(dependencies dependencies) *cobra.Command {
	return newConfirmedClusterCommand(dependencies, confirmedClusterSpec{
		use: "promote <node>", short: "Promote a node",
		example: "  ferric cluster promote node-b --yes", operation: "promote cluster node", args: cobra.ExactArgs(1),
		run: func(ctx context.Context, client connection.Client, args []string) (any, error) {
			operator, err := requireClientCapability[clusterPromoter](client, "cluster promotion")
			if err != nil {
				return nil, err
			}
			return operator.ClusterPromote(ctx, args[0])
		},
	})
}

type clusterDemoter interface {
	ClusterDemote(context.Context, string) (bool, error)
}

func newClusterDemoteCommand(dependencies dependencies) *cobra.Command {
	return newConfirmedClusterCommand(dependencies, confirmedClusterSpec{
		use: "demote <node>", short: "Demote a node",
		example: "  ferric cluster demote node-a --yes", operation: "demote cluster node", args: cobra.ExactArgs(1),
		run: func(ctx context.Context, client connection.Client, args []string) (any, error) {
			operator, err := requireClientCapability[clusterDemoter](client, "cluster demotion")
			if err != nil {
				return nil, err
			}
			return operator.ClusterDemote(ctx, args[0])
		},
	})
}

type confirmedClusterSpec struct {
	use       string
	short     string
	example   string
	operation string
	args      cobra.PositionalArgs
	addFlags  func(*cobra.Command)
	run       func(context.Context, connection.Client, []string) (any, error)
}

func newConfirmedClusterCommand(dependencies dependencies, spec confirmedClusterSpec) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:     spec.use,
		Short:   spec.short,
		Long:    spec.short + ". This changes cluster topology and requires --yes.",
		Example: spec.example,
		Args:    spec.args,
		RunE: func(command *cobra.Command, args []string) error {
			if !yes {
				return errors.New(strings.Split(spec.use, " ")[0] + " requires --yes")
			}
			return runNetworkCommand(command, dependencies, spec.operation, func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				return spec.run(ctx, client, args)
			})
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "confirm the cluster topology change")
	if spec.addFlags != nil {
		spec.addFlags(command)
	}
	return command
}
