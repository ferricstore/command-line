package cli

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ferricstore/command-line/internal/buildinfo"
	ferricstore "github.com/ferricstore/ferricstore-go"
)

type workflowTestClient struct {
	createOptions     ferricstore.CreateOptions
	signalOptions     ferricstore.SignalOptions
	transitionOptions ferricstore.TransitionOptions
	searchOptions     ferricstore.SearchOptions
	closed            bool
}

func (*workflowTestClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (client *workflowTestClient) Close() error {
	client.closed = true
	return nil
}
func (client *workflowTestClient) Create(_ context.Context, options ferricstore.CreateOptions) (*ferricstore.FlowRecord, error) {
	client.createOptions = options
	return &ferricstore.FlowRecord{ID: options.ID, Type: options.Type, State: options.State, Version: 1}, nil
}
func (client *workflowTestClient) Signal(_ context.Context, options ferricstore.SignalOptions) (any, error) {
	client.signalOptions = options
	return map[string]any{"id": options.ID, "signal": options.Signal}, nil
}
func (client *workflowTestClient) Transition(_ context.Context, options ferricstore.TransitionOptions) (*ferricstore.FlowRecord, error) {
	client.transitionOptions = options
	return &ferricstore.FlowRecord{ID: options.ID, Type: "order", State: options.ToState, Version: 2}, nil
}
func (client *workflowTestClient) Search(_ context.Context, options ferricstore.SearchOptions) ([]ferricstore.FlowRecord, error) {
	client.searchOptions = options
	return []ferricstore.FlowRecord{{ID: "order-42", Type: options.Type, State: options.State, Version: 2}}, nil
}

func TestWorkflowStartForwardsLineageAndStructuredPayload(t *testing.T) {
	t.Parallel()

	client := &workflowTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "start", "child", "child-42", `{"step":1}`, "--json",
		"--parent", "order-42", "--root", "order-42", "--correlation-id", "checkout-42", "--partition", "tenant-a",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	options := client.createOptions
	if options.ParentFlowID != "order-42" || options.RootFlowID != "order-42" || options.CorrelationID != "checkout-42" || options.PartitionKey != "tenant-a" {
		t.Fatalf("create options = %#v", options)
	}
	if !reflect.DeepEqual(options.Payload, map[string]any{"step": float64(1)}) {
		t.Fatalf("payload = %#v", options.Payload)
	}
	if !client.closed {
		t.Fatal("client was not closed")
	}
}

func TestWorkflowSignalMapsDataValuesAndGuard(t *testing.T) {
	t.Parallel()

	client := &workflowTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "signal", "order-42", "approved", `{"by":"ada"}`, "--json",
		"--idempotency-key", "approval-7", "--if-state", "review", "--transition-to", "approved", "--attribute", "approver=ada",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	options := client.signalOptions
	if options.ID != "order-42" || options.Signal != "approved" || options.IdempotencyKey != "approval-7" || options.TransitionTo != "approved" {
		t.Fatalf("signal options = %#v", options)
	}
	if !reflect.DeepEqual(options.IfStates, []string{"review"}) {
		t.Fatalf("if states = %#v", options.IfStates)
	}
	if !reflect.DeepEqual(options.Values["data"], map[string]any{"by": "ada"}) {
		t.Fatalf("signal values = %#v", options.Values)
	}
	if !reflect.DeepEqual(options.AttributesMerge, map[string]any{"approver": "ada"}) {
		t.Fatalf("attribute merge = %#v", options.AttributesMerge)
	}
}

func TestWorkflowTransitionUsesExpectedStatesAndFencing(t *testing.T) {
	t.Parallel()

	client := &workflowTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "transition", "order-42", "review", "approved",
		"--lease-token", "lease", "--fencing-token", "3", "--attribute", "approver=ada", "--delete-attribute", "reviewer",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	options := client.transitionOptions
	if options.FromState != "review" || options.ToState != "approved" || options.LeaseToken != "lease" || options.FencingToken != 3 || !options.ReturnRecord {
		t.Fatalf("transition options = %#v", options)
	}
	if !reflect.DeepEqual(options.AttributesMerge, map[string]any{"approver": "ada"}) || !reflect.DeepEqual(options.AttributesDelete, []string{"reviewer"}) {
		t.Fatalf("transition attributes = %#v / %#v", options.AttributesMerge, options.AttributesDelete)
	}
}

func TestWorkflowSearchBuildsBoundedAttributeQuery(t *testing.T) {
	t.Parallel()

	client := &workflowTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{
		"--profile", "production", "workflow", "search", "--type", "order", "--state", "failed",
		"--partition", "tenant-a", "--attribute", "region=eu", "--limit", "20", "--reverse",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	options := client.searchOptions
	if options.Type != "order" || options.State != "failed" || options.PartitionKey != "tenant-a" || options.Count == nil || *options.Count != 20 || options.Rev == nil || !*options.Rev {
		t.Fatalf("search options = %#v", options)
	}
	if !reflect.DeepEqual(options.Attributes, map[string]any{"region": "eu"}) {
		t.Fatalf("attributes = %#v", options.Attributes)
	}
	if !strings.Contains(output.String(), `"id": "order-42"`) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestEveryWorkflowLeafHasDescriptionAndExample(t *testing.T) {
	t.Parallel()

	assertLeafHelp(t, newWorkflowCommand(dependencies{runtime: &runtimeOptions{timeout: time.Second}}))
}
