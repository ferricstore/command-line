// Package cli defines the ferric command-line interface.
package cli

import (
	"fmt"

	"github.com/ferricstore/command-line/internal/buildinfo"
	"github.com/spf13/cobra"
)

// Execute runs the root command.
func Execute(info buildinfo.Info) error {
	return New(info).Execute()
}

// New constructs a command tree without global state.
func New(info buildinfo.Info) *cobra.Command {
	root := &cobra.Command{
		Use:           "ferric",
		Short:         "Manage FerricStore from the command line",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.AddCommand(newVersionCommand(info))
	return root
}

func newVersionCommand(info buildinfo.Info) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), info.String())
			return err
		},
	}
}
