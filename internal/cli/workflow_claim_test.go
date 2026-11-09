package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/buildinfo"
	ferricstore "github.com/ferricstore/ferricstore-go"
)

// workflowClaimExecutor answers FLOW.CLAIM_DUE with one full Flow record, so a
// real SDK client validates the options the command builds.
type workflowClaimExecutor struct {
	calls [][]any
}

func (executor *workflowClaimExecutor) Do(_ context.Context, args ...any) (any, error) {
	executor.calls = append(executor.calls, append([]any(nil), args...))
	return []any{map[string]any{
		"id":            "order-42",
		"type":          "order",
		"state":         "running",
		"run_state":     "queued",
		"partition_key": "tenant-1",
		"lease_token":   "lease",
		"fencing_token": int64(7),
	}}, nil
}

func TestWorkflowClaimSendsOptionsTheSDKAccepts(t *testing.T) {
	t.Parallel()

	executor := &workflowClaimExecutor{}
	client := ferricstore.NewClientWithExecutor(executor)
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{
		"--profile", "production", "workflow", "claim", "order", "--state", "queued", "--worker", "order-worker-1",
	})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v; output = %q", err, output.String())
	}
	if len(executor.calls) != 1 || executor.calls[0][0] != "FLOW.CLAIM_DUE" {
		t.Fatalf("commands = %#v", executor.calls)
	}
	for _, field := range []string{`"id": "order-42"`, `"lease_token": "lease"`, `"fencing_token": 7`} {
		if !strings.Contains(output.String(), field) {
			t.Fatalf("output %q is missing %s", output.String(), field)
		}
	}
}
