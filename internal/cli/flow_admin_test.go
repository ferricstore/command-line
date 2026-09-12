package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/buildinfo"
	ferricstore "github.com/ferricstore/ferricstore-go"
)

type flowAdminClient struct {
	countType       string
	countState      string
	countOptions    ferricstore.ReadOptions
	existsType      string
	existsOptions   ferricstore.ReadOptions
	attributesType  string
	attribute       string
	readOptions     ferricstore.ReadOptions
	ledgerID        string
	ledgerOptions   ferricstore.GovernanceLedgerOptions
	retention       ferricstore.RetentionCleanupOptions
	retentionCalled bool
}

func (*flowAdminClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (*flowAdminClient) Close() error                                    { return nil }

func (client *flowAdminClient) CountByState(_ context.Context, flowType, state string, options ferricstore.ReadOptions) (int64, error) {
	client.countType, client.countState, client.countOptions = flowType, state, options
	return 12, nil
}

func (client *flowAdminClient) Exists(_ context.Context, flowType string, options ferricstore.ReadOptions) (bool, error) {
	client.existsType, client.existsOptions = flowType, options
	return true, nil
}

func (client *flowAdminClient) Attributes(_ context.Context, flowType string, options ferricstore.ReadOptions) ([]map[string]any, error) {
	client.attributesType, client.readOptions = flowType, options
	return []map[string]any{{"name": "region", "count": int64(2)}}, nil
}

func (client *flowAdminClient) AttributeValues(_ context.Context, flowType, attribute string, options ferricstore.ReadOptions) ([]map[string]any, error) {
	client.attributesType, client.attribute, client.readOptions = flowType, attribute, options
	return []map[string]any{{"value": "eu", "count": int64(2)}}, nil
}

func (client *flowAdminClient) GovernanceLedger(_ context.Context, id string, options ferricstore.GovernanceLedgerOptions) ([]map[string]any, error) {
	client.ledgerID, client.ledgerOptions = id, options
	return []map[string]any{{"event": "reserved"}}, nil
}

func (client *flowAdminClient) RetentionCleanup(_ context.Context, options ferricstore.RetentionCleanupOptions) (map[string]any, error) {
	client.retentionCalled, client.retention = true, options
	return map[string]any{"deleted": int64(3)}, nil
}

func TestWorkflowFlowIndexCommandsForwardReadOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		args   []string
		assert func(*testing.T, *flowAdminClient)
	}{
		{
			name: "exists",
			args: []string{"workflow", "exists", "order", "--partition", "tenant-a", "--state", "running"},
			assert: func(t *testing.T, client *flowAdminClient) {
				if client.existsType != "order" || client.existsOptions.PartitionKey != "tenant-a" || client.existsOptions.State != "running" {
					t.Fatalf("exists = %q %#v", client.existsType, client.existsOptions)
				}
			},
		},
		{
			name: "count by state",
			args: []string{"workflow", "count-by-state", "order", "failed", "--partition", "tenant-a", "--include-cold"},
			assert: func(t *testing.T, client *flowAdminClient) {
				if client.countType != "order" || client.countState != "failed" || client.countOptions.PartitionKey != "tenant-a" ||
					client.countOptions.IncludeCold == nil || !*client.countOptions.IncludeCold {
					t.Fatalf("count = %q %q %#v", client.countType, client.countState, client.countOptions)
				}
			},
		},
		{
			name: "attributes",
			args: []string{"workflow", "attributes", "order", "--partition", "tenant-a", "--limit", "25", "--reverse"},
			assert: func(t *testing.T, client *flowAdminClient) {
				if client.attributesType != "order" || client.readOptions.Count == nil || *client.readOptions.Count != 25 ||
					client.readOptions.Rev == nil || !*client.readOptions.Rev {
					t.Fatalf("attributes = %q %#v", client.attributesType, client.readOptions)
				}
			},
		},
		{
			name: "attribute values",
			args: []string{"workflow", "attribute-values", "order", "region", "--partition", "tenant-a", "--state", "running"},
			assert: func(t *testing.T, client *flowAdminClient) {
				if client.attributesType != "order" || client.attribute != "region" || client.readOptions.State != "running" {
					t.Fatalf("attribute values = %q %q %#v", client.attributesType, client.attribute, client.readOptions)
				}
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &flowAdminClient{}
			command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
			command.SetArgs(append([]string{"--profile", "production"}, test.args...))
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			test.assert(t, client)
		})
	}
}

func TestWorkflowGovernanceLedgerForwardsExactContract(t *testing.T) {
	t.Parallel()

	client := &flowAdminClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "governance", "ledger", "order-42",
		"--partition", "tenant-a", "--limit", "15", "--from-ms", "100", "--to-ms", "200", "--reverse",
	})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	options := client.ledgerOptions
	if client.ledgerID != "order-42" || options.PartitionKey != "tenant-a" || options.Limit == nil || *options.Limit != 15 ||
		options.FromMS == nil || *options.FromMS != 100 || options.ToMS == nil || *options.ToMS != 200 || options.Rev == nil || !*options.Rev {
		t.Fatalf("ledger = %q %#v", client.ledgerID, options)
	}
}

func TestWorkflowRetentionCleanupRequiresConfirmationAndForwardsExplicitZero(t *testing.T) {
	t.Parallel()

	client := &flowAdminClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "workflow", "retention", "cleanup", "--limit", "10", "--now-ms", "0"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.retentionCalled {
		t.Fatal("retention cleanup ran without confirmation")
	}

	client = &flowAdminClient{}
	command = New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "workflow", "retention", "cleanup", "--limit", "10", "--now-ms", "0", "--yes"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !client.retentionCalled || client.retention.Limit == nil || *client.retention.Limit != 10 ||
		client.retention.NowMS == nil || *client.retention.NowMS != 0 {
		t.Fatalf("retention = called:%t options:%#v", client.retentionCalled, client.retention)
	}
}
