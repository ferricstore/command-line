package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestFileStoreRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "config.json")
	store := NewFileStore(path)
	want := Profile{
		Name: "production",
		URL:  "ferric://store.example.com:6388",
		Authentication: Authentication{
			Method:   AuthMethodPassword,
			Username: "operator",
		},
	}
	if err := store.Put(context.Background(), want); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	got, err := store.Get(context.Background(), want.Name)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != want {
		t.Fatalf("Get() = %#v, want %#v", got, want)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data document
	if err := json.Unmarshal(contents, &data); err != nil {
		t.Fatal(err)
	}
	if data.CurrentProfile != want.Name {
		t.Fatalf("current profile = %q, want %q", data.CurrentProfile, want.Name)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if gotMode := info.Mode().Perm(); gotMode != 0o600 {
			t.Fatalf("profile mode = %o, want 600", gotMode)
		}
	}
}

func TestFileStoreReplacesProfileWithoutChangingCurrent(t *testing.T) {
	t.Parallel()

	store := NewFileStore(filepath.Join(t.TempDir(), "config.json"))
	ctx := context.Background()
	first := Profile{Name: "first", URL: "ferric://first:6388"}
	second := Profile{Name: "second", URL: "ferric://second:6388"}
	if err := store.Put(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, second); err != nil {
		t.Fatal(err)
	}
	first.URL = "ferric://updated:6388"
	if err := store.Put(ctx, first); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, "first")
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != first.URL {
		t.Fatalf("updated URL = %q, want %q", got.URL, first.URL)
	}
}

func TestFileStoreMissingProfile(t *testing.T) {
	t.Parallel()

	store := NewFileStore(filepath.Join(t.TempDir(), "config.json"))
	_, err := store.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestFileStoreManagesCurrentProfile(t *testing.T) {
	t.Parallel()

	store := NewFileStore(filepath.Join(t.TempDir(), "config.json"))
	ctx := context.Background()
	for _, value := range []Profile{
		{Name: "staging", URL: "ferric://staging:6388"},
		{Name: "default", URL: "ferric://default:6388"},
		{Name: "production", URL: "ferric://production:6388"},
	} {
		if err := store.Put(ctx, value); err != nil {
			t.Fatal(err)
		}
	}

	profiles, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 3 || profiles[0].Name != "default" || profiles[1].Name != "production" || profiles[2].Name != "staging" {
		t.Fatalf("List() = %#v", profiles)
	}
	current, err := store.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if current != "staging" {
		t.Fatalf("Current() = %q, want first profile", current)
	}

	if err := store.Use(ctx, "production"); err != nil {
		t.Fatal(err)
	}
	current, err = store.Current(ctx)
	if err != nil || current != "production" {
		t.Fatalf("Current() after Use = %q, %v", current, err)
	}
	if err := store.Delete(ctx, "production"); err != nil {
		t.Fatal(err)
	}
	current, err = store.Current(ctx)
	if err != nil || current != "default" {
		t.Fatalf("Current() after Delete = %q, %v", current, err)
	}
	if _, err := store.Get(ctx, "production"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(deleted) error = %v", err)
	}
}

func TestFileStoreCurrentWithoutProfiles(t *testing.T) {
	t.Parallel()

	store := NewFileStore(filepath.Join(t.TempDir(), "config.json"))
	if _, err := store.Current(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Current() error = %v, want ErrNotFound", err)
	}
	if err := store.Use(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Use() error = %v, want ErrNotFound", err)
	}
	if err := store.Delete(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestFileStoreConcurrentInstancesDoNotLoseProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	const writers = 64
	start := make(chan struct{})
	errorsByWriter := make(chan error, writers)
	var group sync.WaitGroup
	group.Add(writers)
	for index := 0; index < writers; index++ {
		index := index
		go func() {
			defer group.Done()
			<-start
			store := NewFileStore(path)
			errorsByWriter <- store.Put(context.Background(), Profile{
				Name: fmt.Sprintf("profile-%02d", index),
				URL:  fmt.Sprintf("ferric://node-%02d:6388", index),
			})
		}()
	}
	close(start)
	group.Wait()
	close(errorsByWriter)
	for err := range errorsByWriter {
		if err != nil {
			t.Fatalf("concurrent Put() error = %v", err)
		}
	}

	profiles, err := NewFileStore(path).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != writers {
		t.Fatalf("List() returned %d profiles after %d concurrent writes", len(profiles), writers)
	}
}

func TestFileStoreMutationLockRespectsContext(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.json")
	fileLock := flock.New(path + ".lock")
	if err := fileLock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := fileLock.Unlock(); err != nil {
			t.Errorf("Unlock() error = %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := NewFileStore(path).Put(ctx, Profile{Name: "production"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Put() error = %v, want context deadline", err)
	}
}

func TestFileStoreCredentialMutationSerializesAcrossInstances(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	storeA := NewFileStore(path)
	storeB := NewFileStore(path)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- storeA.WithCredentialMutation(context.Background(), func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first credential mutation did not acquire the lock")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := storeB.WithCredentialMutation(ctx, func(context.Context) error {
		return errors.New("second mutation unexpectedly ran")
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WithCredentialMutation() error = %v, want context deadline", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
