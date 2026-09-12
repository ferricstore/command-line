package cli

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/buildinfo"
	ferricstore "github.com/ferricstore/ferricstore-go"
)

type flowBulkClient struct {
	create     ferricstore.CreateManyOptions
	enqueue    ferricstore.CreateManyOptions
	complete   ferricstore.CompleteManyOptions
	transition ferricstore.TransitionManyOptions
	retry      ferricstore.RetryManyOptions
	fail       ferricstore.FailManyOptions
	cancel     ferricstore.CancelManyOptions
	runSteps   ferricstore.RunStepsManyOptions
}

func (*flowBulkClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (*flowBulkClient) Close() error                                    { return nil }

func (client *flowBulkClient) CreateMany(_ context.Context, options ferricstore.CreateManyOptions) ([]ferricstore.FlowRecord, error) {
	client.create = options
	return []ferricstore.FlowRecord{{ID: options.Items[0].ID, Type: options.Type}}, nil
}

func (client *flowBulkClient) EnqueueMany(_ context.Context, options ferricstore.CreateManyOptions) ([]ferricstore.FlowRecord, error) {
	client.enqueue = options
	return []ferricstore.FlowRecord{{ID: options.Items[0].ID, Type: options.Type}}, nil
}

func (client *flowBulkClient) CompleteMany(_ context.Context, options ferricstore.CompleteManyOptions) ([]ferricstore.FlowRecord, error) {
	client.complete = options
	return []ferricstore.FlowRecord{{ID: options.Items[0].ID, State: "completed"}}, nil
}

func (client *flowBulkClient) TransitionMany(_ context.Context, options ferricstore.TransitionManyOptions) ([]ferricstore.FlowRecord, error) {
	client.transition = options
	return []ferricstore.FlowRecord{{ID: options.Items[0].ID, State: options.ToState}}, nil
}

func (client *flowBulkClient) RetryMany(_ context.Context, options ferricstore.RetryManyOptions) ([]ferricstore.FlowRecord, error) {
	client.retry = options
	return []ferricstore.FlowRecord{{ID: options.Items[0].ID, State: "queued"}}, nil
}

func (client *flowBulkClient) FailMany(_ context.Context, options ferricstore.FailManyOptions) ([]ferricstore.FlowRecord, error) {
	client.fail = options
	return []ferricstore.FlowRecord{{ID: options.Items[0].ID, State: "failed"}}, nil
}

func (client *flowBulkClient) CancelMany(_ context.Context, options ferricstore.CancelManyOptions) ([]ferricstore.FlowRecord, error) {
	client.cancel = options
	return []ferricstore.FlowRecord{{ID: options.Items[0].ID, State: "cancelled"}}, nil
}

func (client *flowBulkClient) RunStepsMany(_ context.Context, options ferricstore.RunStepsManyOptions) error {
	client.runSteps = options
	return nil
}

func writeFlowBulkInput(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFlowCreateManyCommandsDecodeSnakeCaseContracts(t *testing.T) {
	t.Parallel()

	request := `{
		"type":"email","state":"queued","partition_key":"tenant-a",
		"idempotent":true,"independent":false,
		"items":[{"id":"email-1","payload":{"to":"ada@example.com"},"attributes":{"region":"eu"}}]
	}`
	tests := []struct {
		name string
		args []string
		got  func(*flowBulkClient) ferricstore.CreateManyOptions
	}{
		{name: "workflow", args: []string{"workflow", "start-many"}, got: func(client *flowBulkClient) ferricstore.CreateManyOptions { return client.create }},
		{name: "queue", args: []string{"queue", "enqueue-many"}, got: func(client *flowBulkClient) ferricstore.CreateManyOptions { return client.enqueue }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &flowBulkClient{}
			command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
			command.SetArgs(append([]string{"--profile", "production"}, append(test.args, "--file", writeFlowBulkInput(t, request))...))
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			options := test.got(client)
			if options.Type != "email" || options.State != "queued" || options.PartitionKey != "tenant-a" ||
				options.Idempotent == nil || !*options.Idempotent || options.Independent == nil || *options.Independent ||
				len(options.Items) != 1 || options.Items[0].ID != "email-1" ||
				!reflect.DeepEqual(options.Items[0].Payload, map[string]any{"to": "ada@example.com"}) {
				t.Fatalf("options = %#v", options)
			}
		})
	}
}

func TestFlowMutationManyCommandsForwardTypedItems(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		service string
		command string
		input   string
		assert  func(*testing.T, *flowBulkClient)
	}{
		{
			name: "complete", service: "queue", command: "complete-many",
			input: `{"partition_key":"tenant-a","result":{"ok":true},"values":{"receipt":"stored"},"items":[{"id":"job-1","lease_token":"lease","fencing_token":7}]}`,
			assert: func(t *testing.T, client *flowBulkClient) {
				if client.complete.Items[0].LeaseToken != "lease" || client.complete.Items[0].FencingToken != 7 ||
					!reflect.DeepEqual(client.complete.Result, map[string]any{"ok": true}) ||
					!reflect.DeepEqual(client.complete.Values, map[string]any{"receipt": "stored"}) {
					t.Fatalf("complete = %#v", client.complete)
				}
			},
		},
		{
			name: "transition", service: "workflow", command: "transition-many",
			input: `{"from_state":"charging","to_state":"shipping","items":[{"id":"order-1","lease_token":"lease","fencing_token":8}]}`,
			assert: func(t *testing.T, client *flowBulkClient) {
				if client.transition.FromState != "charging" || client.transition.ToState != "shipping" || client.transition.Items[0].FencingToken != 8 {
					t.Fatalf("transition = %#v", client.transition)
				}
			},
		},
		{
			name: "retry", service: "workflow", command: "retry-many",
			input: `{"error":"timeout","items":[{"id":"order-1","lease_token":"lease","fencing_token":9}]}`,
			assert: func(t *testing.T, client *flowBulkClient) {
				if client.retry.Error != "timeout" || client.retry.Items[0].FencingToken != 9 {
					t.Fatalf("retry = %#v", client.retry)
				}
			},
		},
		{
			name: "fail", service: "queue", command: "fail-many",
			input: `{"error":{"code":"exhausted"},"items":[{"id":"job-1","lease_token":"lease","fencing_token":10}]}`,
			assert: func(t *testing.T, client *flowBulkClient) {
				if client.fail.Items[0].FencingToken != 10 || !reflect.DeepEqual(client.fail.Error, map[string]any{"code": "exhausted"}) {
					t.Fatalf("fail = %#v", client.fail)
				}
			},
		},
		{
			name: "cancel", service: "workflow", command: "cancel-many",
			input: `{"reason":"obsolete","items":[{"id":"order-1","fencing_token":11}]}`,
			assert: func(t *testing.T, client *flowBulkClient) {
				if client.cancel.Reason != "obsolete" || client.cancel.Items[0].FencingToken != 11 {
					t.Fatalf("cancel = %#v", client.cancel)
				}
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &flowBulkClient{}
			command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
			command.SetArgs([]string{"--profile", "production", test.service, test.command, "--file", writeFlowBulkInput(t, test.input)})
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			test.assert(t, client)
		})
	}
}

func TestWorkflowRunStepsManyForwardsBatch(t *testing.T) {
	t.Parallel()

	client := &flowBulkClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "run-steps-many", "--file",
		writeFlowBulkInput(t, `{"type":"order","steps":3,"worker":"runner","lease_ms":30000,"items":[{"id":"order-1","partition_key":"tenant-a"}]}`),
	})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.runSteps.Type != "order" || client.runSteps.Steps != 3 || client.runSteps.Worker != "runner" ||
		client.runSteps.LeaseMS != 30000 || len(client.runSteps.Items) != 1 || client.runSteps.Items[0].PartitionKey != "tenant-a" {
		t.Fatalf("run steps = %#v", client.runSteps)
	}
}

func TestFlowBulkInputRejectsUnknownFieldsBeforeConnecting(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "start-many", "--file",
		writeFlowBulkInput(t, `{"type":"order","items":[],"partiton_key":"typo"}`),
	})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.closed {
		t.Fatal("invalid batch input opened a connection")
	}
}

func TestFlowBulkCommandsRejectInvalidContractsBeforeConnecting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		service string
		command string
		input   string
		want    string
	}{
		{
			name: "empty creation batch", service: "workflow", command: "start-many",
			input: `{"type":"order","items":[]}`, want: "at least one item",
		},
		{
			name: "missing creation type", service: "queue", command: "enqueue-many",
			input: `{"items":[{"id":"job-1"}]}`, want: "type is required",
		},
		{
			name: "duplicate creation id", service: "workflow", command: "start-many",
			input: `{"type":"order","items":[{"id":"same"},{"id":"same"}]}`, want: "duplicate",
		},
		{
			name: "missing claim token", service: "queue", command: "complete-many",
			input: `{"items":[{"id":"job-1","fencing_token":1}]}`, want: "lease_token is required",
		},
		{
			name: "invalid transition state", service: "workflow", command: "transition-many",
			input: `{"from_state":"queued","items":[{"id":"order-1","lease_token":"lease","fencing_token":1}]}`, want: "to_state is required",
		},
		{
			name: "ambiguous step selection", service: "workflow", command: "run-steps-many",
			input: `{"type":"order","states":["queued"],"steps":2,"worker":"runner","items":[{"id":"order-1"}]}`, want: "exactly one",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &cliConnectionClient{}
			command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
			command.SetArgs([]string{
				"--profile", "production", test.service, test.command,
				"--file", writeFlowBulkInput(t, test.input),
			})
			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute() error = %v, want %q", err, test.want)
			}
			if client.closed {
				t.Fatal("invalid batch contract opened a connection")
			}
		})
	}
}
