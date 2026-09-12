package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ferricstore/command-line/internal/credential"
	"github.com/ferricstore/command-line/internal/endpoint"
	"github.com/ferricstore/command-line/internal/profile"
	"github.com/spf13/cobra"
)

const profileCredentialRollbackTimeout = 5 * time.Second

func newProfileCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "profile",
		Short:   "Manage saved connections",
		Example: "  ferric profile list\n  ferric profile use production\n  ferric profile show",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(newProfileListCommand(dependencies))
	command.AddCommand(newProfileShowCommand(dependencies))
	command.AddCommand(newProfileUseCommand(dependencies))
	command.AddCommand(newProfileDeleteCommand(dependencies))
	return command
}

func newProfileListCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List saved connections",
		Example: "  ferric profile list",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if dependencies.profiles == nil {
				return errors.New("profile management is not configured")
			}
			profiles, err := dependencies.profiles.List(command.Context())
			if err != nil {
				return fmt.Errorf("list profiles: %w", err)
			}
			if len(profiles) == 0 {
				_, err = fmt.Fprintln(command.OutOrStdout(), "No saved connections.")
				return err
			}
			current, err := dependencies.profiles.Current(command.Context())
			if err != nil && !errors.Is(err, profile.ErrNotFound) {
				return fmt.Errorf("read current profile: %w", err)
			}

			writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(writer, "CURRENT\tNAME\tENDPOINT\tAUTHENTICATION"); err != nil {
				return err
			}
			for _, storedProfile := range profiles {
				marker := ""
				if storedProfile.Name == current {
					marker = "*"
				}
				if _, err := fmt.Fprintf(
					writer,
					"%s\t%s\t%s\t%s\n",
					marker,
					storedProfile.Name,
					profileEndpoint(storedProfile),
					storedProfile.Authentication.Method,
				); err != nil {
					return err
				}
			}
			return writer.Flush()
		},
	}
}

func newProfileShowCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "show [name]",
		Short:   "Show a saved connection without its secret",
		Example: "  ferric profile show\n  ferric profile show production",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if dependencies.profiles == nil {
				return errors.New("profile management is not configured")
			}
			var name string
			if len(args) == 1 {
				name = args[0]
			} else {
				var err error
				name, err = selectedProfile(command, dependencies)
				if err != nil {
					return err
				}
			}
			storedProfile, err := dependencies.profiles.Get(command.Context(), name)
			if err != nil {
				return fmt.Errorf("load profile %q: %w", name, err)
			}
			return writeProfile(command, storedProfile)
		},
	}
	command.ValidArgsFunction = profileCompletion(dependencies)
	return command
}

func writeProfile(command *cobra.Command, storedProfile profile.Profile) error {
	writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
	fields := [][2]string{
		{"Name", storedProfile.Name},
		{"URL", storedProfile.URL},
		{"CA certificate", storedProfile.CACertFile},
		{"Control URL", storedProfile.ControlURL},
		{"Organization", storedProfile.Organization},
		{"Cluster", storedProfile.Cluster},
		{"Authentication", string(storedProfile.Authentication.Method)},
		{"Username", storedProfile.Authentication.Username},
	}
	for _, field := range fields {
		if field[1] == "" {
			continue
		}
		if _, err := fmt.Fprintf(writer, "%s:\t%s\n", field[0], field[1]); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func newProfileUseCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "use <name>",
		Short:   "Select the connection used by default",
		Example: "  ferric profile use production",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if dependencies.profiles == nil {
				return errors.New("profile management is not configured")
			}
			name := strings.TrimSpace(args[0])
			if err := dependencies.profiles.Use(command.Context(), name); err != nil {
				return fmt.Errorf("select profile %q: %w", name, err)
			}
			_, err := fmt.Fprintf(command.OutOrStdout(), "Default connection is now %q.\n", name)
			return err
		},
	}
	command.ValidArgsFunction = profileCompletion(dependencies)
	return command
}

func newProfileDeleteCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "delete <name>",
		Short:   "Delete a saved connection and its credential",
		Long:    "Delete local profile metadata and remove its secret from the operating-system keystore.",
		Example: "  ferric profile delete retired-cluster",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if err := deleteProfile(command.Context(), dependencies, name); err != nil {
				return err
			}
			_, err := fmt.Fprintf(command.OutOrStdout(), "Deleted profile %q.\n", name)
			return err
		},
	}
	command.ValidArgsFunction = profileCompletion(dependencies)
	return command
}

func deleteProfile(ctx context.Context, dependencies dependencies, name string) error {
	if dependencies.profiles == nil || dependencies.credentials == nil {
		return errors.New("profile deletion is not configured")
	}
	if name == "" {
		return errors.New("profile name is required")
	}
	operation := func(mutationContext context.Context) error {
		return deleteProfileState(mutationContext, dependencies, name)
	}
	if coordinator, ok := dependencies.profiles.(profile.CredentialMutationCoordinator); ok {
		return coordinator.WithCredentialMutation(ctx, operation)
	}
	return operation(ctx)
}

func deleteProfileState(ctx context.Context, dependencies dependencies, name string) error {
	storedProfile, err := dependencies.profiles.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("load profile %q: %w", name, err)
	}
	credentialReference := storedProfile.CredentialReference()

	secret, credentialErr := dependencies.credentials.Get(ctx, credentialReference)
	hadCredential := credentialErr == nil
	if credentialErr != nil && !errors.Is(credentialErr, credential.ErrNotFound) {
		return fmt.Errorf("load credential for profile %q: %w", name, credentialErr)
	}
	if hadCredential {
		if err := dependencies.credentials.Delete(ctx, credentialReference); err != nil {
			if !errors.Is(err, credential.ErrNotFound) {
				return fmt.Errorf("delete credential for profile %q: %w", name, err)
			}
			hadCredential = false
		}
	}
	if err := dependencies.profiles.Delete(ctx, name); err != nil {
		profileErr := fmt.Errorf("delete profile %q: %w", name, err)
		if hadCredential {
			cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), profileCredentialRollbackTimeout)
			defer cancel()
			rollbackErr := dependencies.credentials.Put(cleanupContext, credentialReference, secret)
			if rollbackErr != nil {
				return errors.Join(profileErr, fmt.Errorf("restore credential: %w", rollbackErr))
			}
		}
		return profileErr
	}
	return nil
}

func profileEndpoint(storedProfile profile.Profile) string {
	if storedProfile.URL != "" {
		return endpoint.Display(storedProfile.URL)
	}
	return endpoint.Display(storedProfile.ControlURL)
}

func mustRegisterProfileFlagCompletion(command *cobra.Command, dependencies dependencies) {
	if err := command.RegisterFlagCompletionFunc(profileFlagName, profileCompletion(dependencies)); err != nil {
		panic(fmt.Sprintf("register profile completion: %v", err))
	}
}

func profileCompletion(dependencies dependencies) cobra.CompletionFunc {
	return func(command *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if dependencies.profiles == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		profiles, err := dependencies.profiles.List(command.Context())
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		candidates := make([]string, 0, len(profiles))
		for _, storedProfile := range profiles {
			if strings.HasPrefix(storedProfile.Name, toComplete) {
				candidates = append(candidates, storedProfile.Name)
			}
		}
		return candidates, cobra.ShellCompDirectiveNoFileComp
	}
}
