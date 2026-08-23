package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ferricstore/command-line/internal/auth"
	"github.com/ferricstore/command-line/internal/ferric"
	"github.com/ferricstore/command-line/internal/profile"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const maxPasswordBytes = 64 * 1024

type passwordReader interface {
	ReadPassword(io.Reader, io.Writer) (string, error)
}

type terminalPasswordReader struct{}

func (terminalPasswordReader) ReadPassword(input io.Reader, output io.Writer) (string, error) {
	file, ok := input.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return "", errors.New("stdin is not a terminal; use --password-stdin")
	}
	if _, err := fmt.Fprint(output, "Password: "); err != nil {
		return "", err
	}
	password, err := term.ReadPassword(int(file.Fd()))
	_, newlineErr := fmt.Fprintln(output)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	if newlineErr != nil {
		return "", newlineErr
	}
	return string(password), nil
}

func newAuthCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "auth",
		Short:   "Authenticate to FerricStore",
		Example: "  ferric auth login\n  ferric auth status\n  ferric auth logout",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(newLoginCommand(dependencies))
	command.AddCommand(newAuthStatusCommand(dependencies))
	command.AddCommand(newLogoutCommand(dependencies))
	return command
}

func newAuthStatusCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:     "status",
		Short:   "Verify the active authentication",
		Example: "  ferric auth status\n  ferric --output json auth status",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if dependencies.connections == nil {
				return errors.New("connection service is not configured")
			}
			timeout := runtimeTimeout(dependencies)
			if timeout <= 0 {
				return errors.New("timeout must be greater than zero")
			}
			ctx, cancel := context.WithTimeout(command.Context(), timeout)
			defer cancel()
			active, err := openActiveConnection(ctx, command, dependencies)
			if err != nil {
				return fmt.Errorf("authentication status: %w", err)
			}
			_, pingErr := active.client.Ping(ctx)
			closeErr := active.client.Close()
			if err := errors.Join(pingErr, closeErr); err != nil {
				return fmt.Errorf("authentication status: %w", err)
			}

			if runtimeOutput(dependencies) != outputAuto {
				return writeResult(command.OutOrStdout(), runtimeOutput(dependencies), map[string]any{
					"authenticated": true,
					"source":        active.source,
					"user":          active.profile.Authentication.Username,
					"endpoint":      profileEndpoint(active.profile),
					"method":        active.profile.Authentication.Method,
				})
			}
			return writeAuthenticationStatus(command, active.profile, active.source)
		},
	}
	return command
}

func writeAuthenticationStatus(
	command *cobra.Command,
	storedProfile profile.Profile,
	source activeConnectionSource,
) error {
	if _, err := fmt.Fprintln(command.OutOrStdout(), "Authenticated"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(command.OutOrStdout(), "Source: %s\n", source); err != nil {
		return err
	}
	if username := storedProfile.Authentication.Username; username != "" {
		if _, err := fmt.Fprintf(command.OutOrStdout(), "User: %s\n", username); err != nil {
			return err
		}
	}
	if endpoint := profileEndpoint(storedProfile); endpoint != "" {
		if _, err := fmt.Fprintf(command.OutOrStdout(), "Endpoint: %s\n", endpoint); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(command.OutOrStdout(), "Method: %s\n", storedProfile.Authentication.Method)
	return err
}

func newLogoutCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "logout",
		Short:   "Remove the locally stored credential",
		Long:    "Remove the selected profile's password or token from the operating-system keystore without deleting profile metadata. Environment credentials must be unset by the caller.",
		Example: "  ferric auth logout",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if dependencies.login == nil {
				return errors.New("authentication service is not configured")
			}
			_, explicit, err := explicitProfile(command)
			if err != nil {
				return err
			}
			if !explicit && dependencies.environment != nil && dependencies.environment.Configured() {
				_, err := fmt.Fprintln(
					command.OutOrStdout(),
					"Environment credentials cannot be logged out; unset the FERRIC_* credential variables.",
				)
				return err
			}
			profileName, err := selectedProfile(command, dependencies)
			if err != nil {
				return err
			}
			result, err := dependencies.login.Logout(command.Context(), profileName)
			if err != nil {
				return err
			}
			message := "Already logged out."
			if result.Removed {
				message = "Logged out."
			}
			_, err = fmt.Fprintln(command.OutOrStdout(), message)
			return err
		},
	}
}

func newLoginCommand(dependencies dependencies) *cobra.Command {
	var (
		rawURL        string
		caCertFile    string
		username      string
		passwordStdin bool
		noStore       bool
	)

	command := &cobra.Command{
		Use:   "login",
		Short: "Validate and store OSS username/password credentials",
		Long:  "Authenticate with an OSS ACL username and password. The password is stored in the operating-system keystore unless --no-store is used.",
		Example: "  ferric auth login --url ferric://127.0.0.1:6388 --username default\n" +
			"  printf '%s\\n' \"$FERRIC_PASSWORD\" | ferric auth login --url https://store.example.com --username operator --ca-cert /etc/ferric/ca.pem --password-stdin",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if dependencies.login == nil {
				return errors.New("login service is not configured")
			}
			timeout := runtimeTimeout(dependencies)
			if timeout <= 0 {
				return errors.New("timeout must be greater than zero")
			}
			profileName, err := selectedProfile(command, dependencies)
			if err != nil {
				return err
			}

			password, err := loginPassword(command, dependencies.passwordReader, passwordStdin)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(command.Context(), timeout)
			defer cancel()

			result, err := dependencies.login.Login(ctx, auth.LoginRequest{
				ProfileName: profileName,
				Method:      profile.AuthMethodPassword,
				URL:         rawURL,
				CACertFile:  caCertFile,
				Username:    username,
				Secret:      password,
				Store:       !noStore,
			})
			if err != nil {
				return err
			}
			if result.Stored {
				_, err = fmt.Fprintf(
					command.OutOrStdout(),
					"Logged in as %s; saved profile %q.\n",
					result.Principal,
					result.Profile.Name,
				)
				return err
			}
			_, err = fmt.Fprintf(
				command.OutOrStdout(),
				"Authenticated as %s; credentials were not stored.\n",
				result.Principal,
			)
			return err
		},
	}

	command.Flags().StringVar(&rawURL, "url", ferric.DefaultURL, "FerricStore ferric://, ferrics://, or https:// URL")
	command.Flags().StringVar(&caCertFile, "ca-cert", "", "PEM CA certificate for ferrics:// or https:// TLS verification")
	command.Flags().StringVar(&username, "username", "default", "OSS ACL username")
	command.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the password from standard input")
	command.Flags().BoolVar(&noStore, "no-store", false, "validate credentials without storing the profile or password")
	return command
}

func loginPassword(command *cobra.Command, reader passwordReader, fromStdin bool) (string, error) {
	if fromStdin {
		return readPassword(command.InOrStdin())
	}
	if reader == nil {
		return "", errors.New("interactive password input is not configured")
	}
	return reader.ReadPassword(command.InOrStdin(), command.ErrOrStderr())
}

func readPassword(input io.Reader) (string, error) {
	contents, err := io.ReadAll(io.LimitReader(input, maxPasswordBytes+1))
	if err != nil {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	if len(contents) > maxPasswordBytes {
		return "", fmt.Errorf("password exceeds %d bytes", maxPasswordBytes)
	}
	password := string(contents)
	password = strings.TrimSuffix(password, "\n")
	password = strings.TrimSuffix(password, "\r")
	return password, nil
}
