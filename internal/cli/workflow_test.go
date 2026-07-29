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
	searchCalls       int
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
	client.searchCalls++
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
		"--partition", "tenant-a", "--attribute", "region=eu", "--state-meta", "review.approver=ada", "--limit", "20", "--reverse",
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
	if !reflect.DeepEqual(options.StateMeta, map[string]map[string]any{"review": {"approver": "ada"}}) {
		t.Fatalf("state metadata = %#v", options.StateMeta)
	}
	if !strings.Contains(output.String(), `"id": "order-42"`) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestWorkflowCollectionQueriesValidateScopeBeforeSDKCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "list partition", args: []string{"workflow", "list", "order"}, want: "partition is required"},
		{name: "search partition", args: []string{"workflow", "search", "--type", "order", "--attribute", "region=eu"}, want: "partition is required"},
		{name: "search predicate", args: []string{"workflow", "search", "--type", "order", "--partition", "tenant-a"}, want: "requires an attribute or state metadata"},
		{name: "state metadata type", args: []string{"workflow", "search", "--partition", "tenant-a", "--state-meta", "review.approver=ada"}, want: "require a concrete workflow type"},
		{name: "terminal state", args: []string{"workflow", "search", "--partition", "tenant-a", "--attribute", "region=eu", "--terminal-only", "--state", "running"}, want: "terminal state must be"},
		{name: "terminal type", args: []string{"workflow", "terminals", "any", "--partition", "tenant-a"}, want: "require a concrete workflow type"},
		{name: "stuck partition", args: []string{"workflow", "stuck", "order"}, want: "partition is required"},
		{name: "stuck type", args: []string{"workflow", "stuck", "any", "--partition", "tenant-a"}, want: "require a concrete workflow type"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &workflowTestClient{}
			command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
			command.SetArgs(append([]string{"--profile", "production"}, test.args...))
			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute() error = %v, want %q", err, test.want)
			}
			if client.searchCalls != 0 {
				t.Fatal("invalid collection query reached the SDK")
			}
		})
	}
}

func TestWorkflowQueryHelpersOnlyExposeSupportedFlags(t *testing.T) {
	t.Parallel()

	root := New(buildinfo.Info{})
	search, _, err := root.Find([]string{"workflow", "search"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"include-cold", "consistent"} {
		if search.Flags().Lookup(name) != nil {
			t.Fatalf("workflow search exposes unsupported --%s", name)
		}
	}
	children, _, err := root.Find([]string{"workflow", "children"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"attribute", "terminal-only", "include-cold", "consistent"} {
		if children.Flags().Lookup(name) != nil {
			t.Fatalf("workflow children exposes unsupported --%s", name)
		}
	}
	stats, _, err := root.Find([]string{"workflow", "stats"})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Flags().Lookup("include-cold") == nil || stats.Flags().Lookup("consistent") == nil {
		t.Fatal("workflow stats lost its administrative cold-read flags")
	}
}

func TestEveryWorkflowLeafHasDescriptionAndExample(t *testing.T) {
	t.Parallel()

	assertLeafHelp(t, newWorkflowCommand(dependencies{runtime: &runtimeOptions{timeout: time.Second}}))
}
