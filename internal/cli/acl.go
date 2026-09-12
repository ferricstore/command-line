package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/profile"
	"github.com/spf13/cobra"
)

func newACLCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "acl",
		Short: "Inspect and manage OSS ACL users",
		Long:  "Manage FerricStore ACL users and rules. ACL traffic uses the selected ferric:// or ferrics:// connection exactly as configured.",
		Example: "  ferric acl whoami\n" +
			"  ferric acl list\n" +
			"  ferric acl get-user operator",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newACLReadCommand(dependencies, "whoami", "whoami", "Show the authenticated ACL user", "  ferric acl whoami", 0, 0),
		newACLReadCommand(dependencies, "list", "list", "List ACL rules for all users", "  ferric acl list", 0, 0),
		newACLReadCommand(dependencies, "get-user", "getuser <username>", "Show one ACL user", "  ferric acl get-user operator", 1, 1),
		newACLSetUserCommand(dependencies),
		newACLDeleteUserCommand(dependencies),
		newACLConfirmedNoArgsCommand(dependencies, "save", "Persist ACL state to its configured file", "SAVE"),
		newACLConfirmedNoArgsCommand(dependencies, "load", "Reload ACL state from its configured file", "LOAD"),
	)
	return command
}

type aclOperator interface {
	ACL(context.Context, string, ...any) (any, error)
}

func runACL(command *cobra.Command, dependencies dependencies, operation, subcommand string, args []string) error {
	return runNetworkCommand(command, dependencies, operation, func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
		operator, err := requireClientCapability[aclOperator](client, "ACL operations")
		if err != nil {
			return nil, err
		}
		arguments := make([]any, len(args))
		for index := range args {
			arguments[index] = args[index]
		}
		return operator.ACL(ctx, subcommand, arguments...)
	})
}

func newACLReadCommand(dependencies dependencies, name, use, short, example string, minArgs, maxArgs int) *cobra.Command {
	return &cobra.Command{
		Use:     strings.Replace(use, strings.Fields(use)[0], name, 1),
		Short:   short,
		Example: example,
		Args: func(command *cobra.Command, args []string) error {
			if len(args) < minArgs || (maxArgs >= 0 && len(args) > maxArgs) {
				return fmt.Errorf("invalid arguments for %s", command.CommandPath())
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			return runACL(command, dependencies, strings.ToLower(name), strings.ToUpper(strings.Fields(use)[0]), args)
		},
	}
}

func newACLSetUserCommand(dependencies dependencies) *cobra.Command {
	var passwordStdin bool
	var yes bool
	command := &cobra.Command{
		Use:   "set-user <username> [rule...]",
		Short: "Create or update an ACL user",
		Long: "Apply Redis-compatible ACL rules. This command requires --yes. Use --password-stdin to avoid putting a new password in shell history. " +
			"A literal >password rule is accepted when explicitly supplied by the user.",
		Example: "  ferric acl set-user --yes reporter on '~reports:*' +get\n" +
			"  printf '%s\\n' \"$NEW_PASSWORD\" | ferric acl set-user --yes --password-stdin reporter on '~reports:*' +get",
		Args: cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := requireProtocolConfirmation(yes, "ACL user update", []any{"ACL", "SETUSER"}); err != nil {
				return err
			}
			rules := append([]string(nil), args[1:]...)
			if passwordStdin {
				password, err := readPassword(command.InOrStdin())
				if err != nil {
					return err
				}
				if password == "" {
					return errors.New("password from stdin is empty")
				}
				rules = append(rules, ">"+password)
			}
			return runACL(command, dependencies, "set ACL user", "SETUSER", append([]string{args[0]}, rules...))
		},
	}
	command.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read a password from stdin and add it as an ACL rule")
	command.Flags().BoolVar(&yes, "yes", false, "confirm the ACL user update")
	command.Flags().SetInterspersed(false)
	return command
}

func newACLDeleteUserCommand(dependencies dependencies) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:     "delete-user <username>...",
		Short:   "Delete one or more ACL users",
		Long:    "Permanently delete ACL users. This command requires --yes.",
		Example: "  ferric acl delete-user retired-operator --yes",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !yes {
				return errors.New("ACL user deletion requires --yes")
			}
			return runACL(command, dependencies, "delete ACL users", "DELUSER", args)
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "confirm permanent user deletion")
	return command
}

func newACLConfirmedNoArgsCommand(dependencies dependencies, name, short, subcommand string) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:     name,
		Short:   short,
		Long:    short + ". This command requires --yes.",
		Example: "  ferric acl " + name + " --yes",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if !yes {
				return errors.New("ACL " + name + " requires --yes")
			}
			return runACL(command, dependencies, name+" ACL state", subcommand, nil)
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "confirm the ACL state operation")
	return command
}
