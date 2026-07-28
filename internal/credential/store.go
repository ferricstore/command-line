// Package credential stores profile secrets outside the profile document.
package credential

import (
	"context"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const keyringService = "ferric"

// ErrNotFound indicates that a profile has no stored credential.
var ErrNotFound = errors.New("credential not found")

// Store persists the secret associated with a profile.
type Store interface {
	Put(context.Context, string, string) error
	Get(context.Context, string) (string, error)
	Delete(context.Context, string) error
}

type keyringBackend interface {
	Set(service, user, password string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

type systemKeyring struct{}

func (systemKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (systemKeyring) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (systemKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

// KeyringStore stores credentials in the operating-system keyring.
type KeyringStore struct {
	backend keyringBackend
}

// NewKeyringStore constructs a system-keyring credential store.
func NewKeyringStore() *KeyringStore {
	return &KeyringStore{backend: systemKeyring{}}
}

// Put stores a profile credential.
func (s *KeyringStore) Put(ctx context.Context, profileName, secret string) error {
	if err := validate(ctx, profileName); err != nil {
		return err
	}
	if err := s.backend.Set(keyringService, account(profileName), secret); err != nil {
		return fmt.Errorf("store credential in system keyring: %w", err)
	}
	return nil
}

// Get retrieves a profile credential.
func (s *KeyringStore) Get(ctx context.Context, profileName string) (string, error) {
	if err := validate(ctx, profileName); err != nil {
		return "", err
	}
	secret, err := s.backend.Get(keyringService, account(profileName))
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read credential from system keyring: %w", err)
	}
	return secret, nil
}

// Delete removes a profile credential.
func (s *KeyringStore) Delete(ctx context.Context, profileName string) error {
	if err := validate(ctx, profileName); err != nil {
		return err
	}
	if err := s.backend.Delete(keyringService, account(profileName)); errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("delete credential from system keyring: %w", err)
	}
	return nil
}

func validate(ctx context.Context, profileName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if profileName == "" {
		return errors.New("profile name is required")
	}
	return nil
}

func account(profileName string) string {
	return "profile:" + profileName
}
