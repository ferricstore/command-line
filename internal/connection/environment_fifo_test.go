//go:build unix

package connection

import (
	"context"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestEnvironmentCredentialSourceRejectsFIFOSecretWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		envURL:          "ferric://127.0.0.1:6388",
		envUsername:     "operator",
		envPasswordFile: path,
	}
	source := newEnvironmentCredentialSource(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}, readBoundedSecretFile)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, _, err := source.Resolve(ctx)
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("Resolve() error = %v, want non-regular secret rejection", err)
	}
	if elapsed := time.Since(started); elapsed >= 200*time.Millisecond {
		t.Fatalf("Resolve() blocked on FIFO for %v", elapsed)
	}
}
