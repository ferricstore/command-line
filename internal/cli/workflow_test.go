package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ferricstore/command-line/internal/buildinfo"
	ferricstore "github.com/ferricstore/ferricstore-go"
)

type workflowReclaimClient struct {
	options ferricstore.ReclaimOptions
	calls   int
}

type workflowStartAndClaimClient struct {
	options ferricstore.StartAndClaimOptions
}

func (*workflowStartAndClaimClient) Ping(context.Context, ...string) (string, error) {
	return "PONG", nil
}
func (*workflowStartAndClaimClient) Close() error { return nil }
func (client *workflowStartAndClaimClient) StartAndClaim(_ context.Context, options ferricstore.StartAndClaimOptions) (*ferricstore.FlowRecord, error) {
	client.options = options
	return &ferricstore.FlowRecord{ID: options.ID, Type: options.Type, State: "running", LeaseToken: "lease", FencingToken: 1}, nil
}

type workflowStepContinueClient struct {
	options ferricstore.StepContinueOptions
}

type workflowSpawnChildrenClient struct {
	options ferricstore.SpawnChildrenOptions
}

type workflowValuePutClient struct {
	name    string
	value   any
	options ferricstore.ValuePutOptions
	upload  bool
}

func (*workflowValuePutClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (*workflowValuePutClient) Close() error                                    { return nil }
func (client *workflowValuePutClient) PutValue(_ context.Context, name string, value any, options ferricstore.ValuePutOptions) (any, error) {
	client.name = name
	client.value = value
	client.options = options
	return map[string]any{"reference": "value-ref-1"}, nil
}
func (client *workflowValuePutClient) ValuePut(_ context.Context, value any, options ferricstore.ValuePutOptions) (any, error) {
	client.upload = true
	client.value = value
	client.options = options
	return map[string]any{"reference": "value-ref-1"}, nil
}

func (*workflowSpawnChildrenClient) Ping(context.Context, ...string) (string, error) {
	return "PONG", nil
}
func (*workflowSpawnChildrenClient) Close() error { return nil }
func (client *workflowSpawnChildrenClient) SpawnChildren(_ context.Context, options ferricstore.SpawnChildrenOptions) (any, error) {
	client.options = options
	return map[string]any{"created": len(options.Children)}, nil
}

func (*workflowStepContinueClient) Ping(context.Context, ...string) (string, error) {
	return "PONG", nil
}
func (*workflowStepContinueClient) Close() error { return nil }
func (client *workflowStepContinueClient) StepContinue(_ context.Context, options ferricstore.StepContinueOptions) (*ferricstore.FlowRecord, error) {
	client.options = options
	return &ferricstore.FlowRecord{ID: options.ID, State: options.ToState, LeaseToken: options.LeaseToken, FencingToken: options.FencingToken}, nil
}

func (*workflowReclaimClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (*workflowReclaimClient) Close() error                                    { return nil }
func (client *workflowReclaimClient) Reclaim(_ context.Context, options ferricstore.ReclaimOptions) ([]ferricstore.FlowRecord, error) {
	client.calls++
	client.options = options
	return []ferricstore.FlowRecord{{ID: "order-42", Type: options.Type, State: "running"}}, nil
}

func TestWorkflowReclaimSupportsValueProjection(t *testing.T) {
	t.Parallel()

	client := &workflowReclaimClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "reclaim", "order", "--worker", "worker-2",
		"--value", "receipt", "--payload=false", "--attributes=false",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 || !reflect.DeepEqual(client.options.Values, []string{"receipt"}) ||
		client.options.Payload == nil || *client.options.Payload ||
		client.options.IncludeAttributes == nil || *client.options.IncludeAttributes {
		t.Fatalf("reclaim options = %#v", client.options)
	}
}

func TestWorkflowStartAndClaimForwardsCreationAndLeaseFields(t *testing.T) {
	t.Parallel()

	client := &workflowStartAndClaimClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "start-and-claim", "order", "order-42", `{"total":125}`,
		"--json", "--worker", "worker-1", "--initial-state", "queued", "--lease", "45s",
		"--partition", "tenant-a", "--attribute", "region=eu",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.options.ID != "order-42" || client.options.Type != "order" || client.options.Worker != "worker-1" ||
		client.options.InitialState != "queued" || client.options.LeaseMS != int64((45*time.Second)/time.Millisecond) ||
		client.options.PartitionKey != "tenant-a" || !reflect.DeepEqual(client.options.Payload, map[string]any{"total": float64(125)}) ||
		!reflect.DeepEqual(client.options.Attributes, map[string]any{"region": "eu"}) {
		t.Fatalf("start-and-claim options = %#v", client.options)
	}
}

func TestWorkflowStepContinueForwardsFenceAndRenewsLease(t *testing.T) {
	t.Parallel()

	client := &workflowStepContinueClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "step-continue", "order-42", "charging", "shipping",
		"--lease-token", "lease", "--fencing-token", "7", "--lease", "1m", "--worker", "worker-2",
		"--attribute", "stage=shipping", "--value", "receipt=stored",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.options.ID != "order-42" || client.options.FromState != "charging" || client.options.ToState != "shipping" ||
		client.options.LeaseToken != "lease" || client.options.FencingToken != 7 ||
		client.options.LeaseMS != int64(time.Minute/time.Millisecond) || client.options.Worker != "worker-2" ||
		!reflect.DeepEqual(client.options.AttributesMerge, map[string]any{"stage": "shipping"}) ||
		!reflect.DeepEqual(client.options.Values, map[string]any{"receipt": "stored"}) {
		t.Fatalf("step-continue options = %#v", client.options)
	}
}

func TestWorkflowSpawnChildrenReadsTypedBatchFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "children.json")
	contents := `[
		{"id":"child-1","type":"charge","payload":{"amount":125},"partition_key":"tenant-a","attributes":{"region":"eu"}},
		{"id":"child-2","type":"notify","payload":{"channel":"email"}}
	]`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &workflowSpawnChildrenClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "spawn-children", "order-42", "--file", path,
		"--lease-token", "lease", "--fencing-token", "7", "--group", "fulfillment", "--wait", "all",
		"--success", "completed", "--failure", "failed", "--partition", "tenant-a",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.options.ID != "order-42" || client.options.GroupID != "fulfillment" || client.options.Wait != "all" ||
		client.options.LeaseToken != "lease" || client.options.FencingToken == nil || *client.options.FencingToken != 7 ||
		len(client.options.Children) != 2 || client.options.Children[0].ID != "child-1" ||
		!reflect.DeepEqual(client.options.Children[0].Payload, map[string]any{"amount": float64(125)}) {
		t.Fatalf("spawn options = %#v", client.options)
	}
}

func TestWorkflowNamedValuePutForwardsOwnership(t *testing.T) {
	t.Parallel()

	client := &workflowValuePutClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "values", "put", "receipt", `{"status":"paid"}`,
		"--json", "--owner", "order-42", "--partition", "tenant-a", "--override",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.name != "receipt" || !reflect.DeepEqual(client.value, map[string]any{"status": "paid"}) ||
		client.options.OwnerFlowID != "order-42" || client.options.PartitionKey != "tenant-a" ||
		client.options.Override == nil || !*client.options.Override {
		t.Fatalf("value put = %q %#v %#v", client.name, client.value, client.options)
	}
}

func TestWorkflowValueUploadForwardsTTL(t *testing.T) {
	t.Parallel()

	client := &workflowValuePutClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "values", "upload", "binary-ish", "--ttl", "24h",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !client.upload || client.value != "binary-ish" || client.options.TTLMS == nil ||
		*client.options.TTLMS != int64((24*time.Hour)/time.Millisecond) {
		t.Fatalf("value upload = %#v %#v", client.value, client.options)
	}
}

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
