// Package profile stores non-secret CLI connection profiles.
package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

const (
	documentVersion = 1
	lockRetryDelay  = 10 * time.Millisecond
)

// AuthMethod identifies how a profile obtains connection credentials.
type AuthMethod string

const (
	// AuthMethodPassword authenticates directly to an OSS cluster.
	AuthMethodPassword AuthMethod = "password"
	// AuthMethodEnterpriseSSO authenticates a human through Enterprise SSO.
	AuthMethodEnterpriseSSO AuthMethod = "enterprise-sso"
	// AuthMethodEnterpriseAPIToken exchanges a machine credential through Enterprise.
	AuthMethodEnterpriseAPIToken AuthMethod = "enterprise-api-token"
)

// Authentication contains non-secret authentication metadata.
type Authentication struct {
	Method   AuthMethod `json:"method"`
	Username string     `json:"username,omitempty"`
}

// Profile describes a named FerricStore connection.
type Profile struct {
	Name           string         `json:"-"`
	URL            string         `json:"url,omitempty"`
	CACertFile     string         `json:"ca_cert_file,omitempty"`
	ControlURL     string         `json:"control_url,omitempty"`
	Organization   string         `json:"organization,omitempty"`
	Cluster        string         `json:"cluster,omitempty"`
	Authentication Authentication `json:"auth"`
}

// Store persists named profiles.
type Store interface {
	Put(context.Context, Profile) error
	Get(context.Context, string) (Profile, error)
}

// Manager provides profile discovery and current-profile selection.
type Manager interface {
	Store
	List(context.Context) ([]Profile, error)
	Delete(context.Context, string) error
	Current(context.Context) (string, error)
	Use(context.Context, string) error
}

// ErrNotFound indicates that a profile does not exist.
var ErrNotFound = errors.New("profile not found")

type document struct {
	Version        int                `json:"version"`
	CurrentProfile string             `json:"current_profile,omitempty"`
	Profiles       map[string]Profile `json:"profiles"`
}

// FileStore stores profiles in a JSON file.
type FileStore struct {
	path    string
	pathErr error
	mu      sync.Mutex
}

// NewFileStore constructs a profile store at path.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// NewDefaultFileStore constructs the platform-default profile store.
func NewDefaultFileStore() *FileStore {
	if directory := os.Getenv("FERRIC_CONFIG_DIR"); directory != "" {
		return NewFileStore(filepath.Join(directory, "config.json"))
	}

	directory, err := os.UserConfigDir()
	if err != nil {
		return &FileStore{pathErr: fmt.Errorf("resolve user config directory: %w", err)}
	}
	return NewFileStore(filepath.Join(directory, "ferric", "config.json"))
}

// Put adds or replaces a named profile.
func (s *FileStore) Put(ctx context.Context, value Profile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if value.Name == "" {
		return errors.New("profile name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.withMutationLock(ctx, func() error {
		data, err := s.load()
		if err != nil {
			return err
		}
		stored := value
		stored.Name = ""
		data.Profiles[value.Name] = stored
		if data.CurrentProfile == "" {
			data.CurrentProfile = value.Name
		}
		return s.save(data)
	})
}

// Get returns a named profile.
func (s *FileStore) Get(ctx context.Context, name string) (Profile, error) {
	if err := ctx.Err(); err != nil {
		return Profile{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return Profile{}, err
	}
	value, ok := data.Profiles[name]
	if !ok {
		return Profile{}, ErrNotFound
	}
	value.Name = name
	return value, nil
}

// List returns every profile ordered by name.
func (s *FileStore) List(ctx context.Context) ([]Profile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return nil, err
	}
	names := profileNames(data.Profiles)
	profiles := make([]Profile, 0, len(names))
	for _, name := range names {
		value := data.Profiles[name]
		value.Name = name
		profiles = append(profiles, value)
	}
	return profiles, nil
}

// Current returns the profile selected for commands without an explicit override.
func (s *FileStore) Current(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return "", err
	}
	if _, ok := data.Profiles[data.CurrentProfile]; data.CurrentProfile != "" && ok {
		return data.CurrentProfile, nil
	}
	if _, ok := data.Profiles["default"]; ok {
		return "default", nil
	}
	names := profileNames(data.Profiles)
	if len(names) == 0 {
		return "", ErrNotFound
	}
	return names[0], nil
}

// Use selects an existing profile for subsequent commands.
func (s *FileStore) Use(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("profile name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.withMutationLock(ctx, func() error {
		data, err := s.load()
		if err != nil {
			return err
		}
		if _, ok := data.Profiles[name]; !ok {
			return ErrNotFound
		}
		data.CurrentProfile = name
		return s.save(data)
	})
}

// Delete removes a profile and chooses a deterministic replacement if needed.
func (s *FileStore) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("profile name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.withMutationLock(ctx, func() error {
		data, err := s.load()
		if err != nil {
			return err
		}
		if _, ok := data.Profiles[name]; !ok {
			return ErrNotFound
		}
		delete(data.Profiles, name)
		if data.CurrentProfile == name {
			data.CurrentProfile = ""
			if _, ok := data.Profiles["default"]; ok {
				data.CurrentProfile = "default"
			} else if names := profileNames(data.Profiles); len(names) > 0 {
				data.CurrentProfile = names[0]
			}
		}
		return s.save(data)
	})
}

func (s *FileStore) withMutationLock(ctx context.Context, operation func() error) (err error) {
	if s.pathErr != nil {
		return s.pathErr
	}
	if s.path == "" {
		return errors.New("profile path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}
	fileLock := flock.New(s.path+".lock", flock.SetPermissions(0o600))
	locked, err := fileLock.TryLockContext(ctx, lockRetryDelay)
	if err != nil {
		return fmt.Errorf("lock profiles: %w", err)
	}
	if !locked {
		if err := ctx.Err(); err != nil {
			return err
		}
		return errors.New("lock profiles: lock was not acquired")
	}
	defer func() {
		if unlockErr := fileLock.Unlock(); unlockErr != nil {
			err = errors.Join(err, fmt.Errorf("unlock profiles: %w", unlockErr))
		}
	}()
	return operation()
}

func profileNames(profiles map[string]Profile) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *FileStore) load() (document, error) {
	if s.pathErr != nil {
		return document{}, s.pathErr
	}
	if s.path == "" {
		return document{}, errors.New("profile path is required")
	}

	contents, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return document{Version: documentVersion, Profiles: make(map[string]Profile)}, nil
	}
	if err != nil {
		return document{}, fmt.Errorf("read profiles: %w", err)
	}

	var data document
	if err := json.Unmarshal(contents, &data); err != nil {
		return document{}, fmt.Errorf("decode profiles: %w", err)
	}
	if data.Version != documentVersion {
		return document{}, fmt.Errorf("unsupported profile document version %d", data.Version)
	}
	if data.Profiles == nil {
		data.Profiles = make(map[string]Profile)
	}
	return data, nil
}

func (s *FileStore) save(data document) error {
	contents, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profiles: %w", err)
	}
	contents = append(contents, '\n')

	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary profile file: %w", err)
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()

	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary profile file: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write profiles: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync profiles: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close profiles: %w", err)
	}
	if err := os.Rename(temporaryName, s.path); err != nil {
		return fmt.Errorf("replace profiles: %w", err)
	}
	removeTemporary = false
	return nil
}
