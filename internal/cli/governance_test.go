package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/ferricstore/command-line/internal/buildinfo"
)

func TestApprovalRequestRejectsConflictingExpiryBeforeConnecting(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "governance", "approval", "request", "approval-42",
		"--flow-id", "order-42", "--scope", "payments", "--timeout-after", time.Minute.String(),
		"--expires-at", "2026-08-01T08:00:00Z",
	})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.closed {
		t.Fatal("conflicting approval expiry flags opened a connection")
	}
}
