package cli

import "github.com/spf13/cobra"

func newPubSubCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "pubsub",
		Short: "Publish messages and inspect Pub/Sub channels",
		Long:  "Publish one message or inspect channel metadata. Long-lived subscribe mode is outside the current non-interactive CLI scope.",
		Example: "  ferric pubsub publish events '{\"type\":\"ready\"}'\n" +
			"  ferric pubsub channels 'events:*'",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error { return command.Help() },
	}
	for _, spec := range []sdkCommandSpec{
		{name: "publish", use: "publish <channel> <message>", short: "Publish one message", example: "  ferric pubsub publish events '{\"type\":\"ready\"}'", wire: []any{"PUBLISH"}, minArgs: 2, maxArgs: 2},
		{name: "channels", use: "channels [pattern]", short: "List active channels", example: "  ferric pubsub channels\n  ferric pubsub channels 'events:*'", wire: []any{"PUBSUB", "CHANNELS"}, minArgs: 0, maxArgs: 1},
		{name: "subscribers", aliases: []string{"numsub"}, use: "subscribers [channel]...", short: "Count subscribers by channel", example: "  ferric pubsub subscribers events alerts", wire: []any{"PUBSUB", "NUMSUB"}, minArgs: 0, maxArgs: -1},
		{name: "patterns", aliases: []string{"numpat"}, use: "patterns", short: "Count active pattern subscriptions", example: "  ferric pubsub patterns", wire: []any{"PUBSUB", "NUMPAT"}, minArgs: 0, maxArgs: 0},
		{name: "spublish", use: "spublish <shard-channel> <message>", short: "Publish one message to a shard channel", example: "  ferric pubsub spublish 'orders:{42}' '{\"type\":\"ready\"}'", wire: []any{"SPUBLISH"}, minArgs: 2, maxArgs: 2},
		{name: "shard-channels", aliases: []string{"shardchannels"}, use: "shard-channels [pattern]", short: "List active shard channels", example: "  ferric pubsub shard-channels\n  ferric pubsub shard-channels 'orders:*'", wire: []any{"PUBSUB", "SHARDCHANNELS"}, minArgs: 0, maxArgs: 1},
		{name: "shard-subscribers", aliases: []string{"shardnumsub"}, use: "shard-subscribers [shard-channel]...", short: "Count subscribers by shard channel", example: "  ferric pubsub shard-subscribers 'orders:{42}'", wire: []any{"PUBSUB", "SHARDNUMSUB"}, minArgs: 0, maxArgs: -1},
	} {
		command.AddCommand(newSDKCommand(dependencies, spec))
	}
	return command
}
