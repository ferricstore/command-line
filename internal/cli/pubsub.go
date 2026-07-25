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
	} {
		command.AddCommand(newSDKCommand(dependencies, spec))
	}
	return command
}
