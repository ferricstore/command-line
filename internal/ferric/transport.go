package ferric

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	ferricstore "github.com/ferricstore/ferricstore-go"
)

// RequireConnectionAffine rejects stateless HTTP endpoints for operations that
// must retain one native connection for their full lifetime.
func RequireConnectionAffine(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("FerricStore URL is invalid")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return ferricstore.ErrHTTPConnectionAffineCommand
	default:
		return nil
	}
}

// NormalizeOperationError turns the SDK's transport sentinel into stable CLI
// guidance without exposing the endpoint or credentials.
func NormalizeOperationError(operation, rawURL string, err error) error {
	if err == nil || !errors.Is(err, ferricstore.ErrHTTPConnectionAffineCommand) {
		return err
	}
	parsed, parseErr := url.Parse(rawURL)
	scheme := "HTTP"
	if parseErr == nil && parsed.Scheme != "" {
		scheme = strings.ToLower(parsed.Scheme) + "://"
	}
	return fmt.Errorf(
		"%s requires ferric:// or ferrics://; the active endpoint uses %s: %w",
		operation,
		scheme,
		ferricstore.ErrHTTPConnectionAffineCommand,
	)
}
