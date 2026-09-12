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
	"github.com/spf13/cobra"
)

type queueEnqueueClient struct {
	options ferricstore.CreateOptions
	closed  bool
}

func (*queueEnqueueClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (client *queueEnqueueClient) Close() error {
	client.closed = true
	return nil
}
func (client *queueEnqueueClient) Enqueue(_ context.Context, options ferricstore.CreateOptions) (*ferricstore.FlowRecord, error) {
	client.options = options
	return &ferricstore.FlowRecord{
		ID:         options.ID,
		Type:       options.Type,
		State:      options.State,
		Payload:    options.Payload,
		Attributes: options.Attributes,
		Version:    1,
	}, nil
}

type queueClaimClient struct {
	options ferricstore.ClaimDueOptions
	calls   int
}

func (*queueClaimClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (*queueClaimClient) Close() error                                    { return nil }
func (client *queueClaimClient) ClaimJobs(_ context.Context, options ferricstore.ClaimDueOptions) ([]ferricstore.ClaimedItem, error) {
	client.calls++
	client.options = options
	return []ferricstore.ClaimedItem{{
		ID:           "email-42",
		Type:         options.Type,
		State:        "queued",
		LeaseToken:   "lease",
		FencingToken: 7,
		Payload:      map[string]any{"to": "ada@example.com"},
	}}, nil
}

type queueCompleteClient struct {
	options ferricstore.CompleteOptions
	calls   int
}

type queueCancelClient struct {
	options ferricstore.CancelOptions
	calls   int
}

type queuePolicyClient struct {
	calls int
}

type queueReclaimClient struct {
	options ferricstore.ReclaimOptions
	calls   int
}

func (*queueReclaimClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (*queueReclaimClient) Close() error                                    { return nil }
func (client *queueReclaimClient) ReclaimJobs(_ context.Context, options ferricstore.ReclaimOptions) ([]ferricstore.ClaimedItem, error) {
	client.calls++
	client.options = options
	return []ferricstore.ClaimedItem{{ID: "email-42", LeaseToken: "new-lease", FencingToken: 8}}, nil
}

func (*queuePolicyClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (*queuePolicyClient) Close() error                                    { return nil }
func (client *queuePolicyClient) SetPolicy(_ context.Context, _ string, _ ferricstore.PolicyOptions) (ferricstore.PolicySnapshot, error) {
	client.calls++
	return ferricstore.PolicySnapshot{}, nil
}

func (*queueCancelClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (*queueCancelClient) Close() error                                    { return nil }
func (client *queueCancelClient) Cancel(_ context.Context, options ferricstore.CancelOptions) (*ferricstore.FlowRecord, error) {
	client.calls++
	client.options = options
	return &ferricstore.FlowRecord{ID: options.ID, Type: "email", State: "cancelled", Version: 2}, nil
}

func (*queueCompleteClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (*queueCompleteClient) Close() error                                    { return nil }
func (client *queueCompleteClient) Complete(_ context.Context, options ferricstore.CompleteOptions) (*ferricstore.FlowRecord, error) {
	client.calls++
	client.options = options
	return &ferricstore.FlowRecord{ID: options.ID, Type: "email", State: "completed", Version: 2}, nil
}

func TestQueueEnqueueParsesJSONAndAttributes(t *testing.T) {
	t.Parallel()

	client := &queueEnqueueClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{
		"--profile", "production", "queue", "enqueue", "email", "email-42", `{"to":"ada@example.com"}`,
		"--json", "--attribute", "tenant=acme", "--priority", "7", "--idempotent",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.options.ID != "email-42" || client.options.Type != "email" || client.options.State != "queued" {
		t.Fatalf("options = %#v", client.options)
	}
	wantPayload := map[string]any{"to": "ada@example.com"}
	if !reflect.DeepEqual(client.options.Payload, wantPayload) {
		t.Fatalf("payload = %#v", client.options.Payload)
	}
	if !reflect.DeepEqual(client.options.Attributes, map[string]any{"tenant": "acme"}) {
		t.Fatalf("attributes = %#v", client.options.Attributes)
	}
	if client.options.Priority == nil || *client.options.Priority != 7 {
		t.Fatalf("priority = %#v", client.options.Priority)
	}
	if client.options.Idempotent == nil || !*client.options.Idempotent {
		t.Fatalf("idempotent = %#v", client.options.Idempotent)
	}
	if !client.closed {
		t.Fatal("client was not closed")
	}
	if !strings.Contains(output.String(), `"id": "email-42"`) || strings.Contains(output.String(), `"raw"`) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestQueueClaimUsesWorkerLeaseAndReceiptFields(t *testing.T) {
	t.Parallel()

	client := &queueClaimClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{
		"--profile", "production", "queue", "receive", "email", "--worker", "mailer-1", "--lease", "45s", "--limit", "3",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.options.Worker != "mailer-1" || client.options.LeaseMS != int64((45*time.Second)/time.Millisecond) || client.options.Limit != 3 {
		t.Fatalf("claim options = %#v", client.options)
	}
	if !client.options.JobOnly || !client.options.IncludeState || client.options.Payload == nil || !*client.options.Payload {
		t.Fatalf("claim projection options = %#v", client.options)
	}
	if !strings.Contains(output.String(), `"lease_token": "lease"`) || !strings.Contains(output.String(), `"fencing_token": 7`) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestQueueClaimValidatesWorkerBeforeConnecting(t *testing.T) {
	t.Parallel()

	client := &queueClaimClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "queue", "claim", "email"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "worker is required") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.calls != 0 {
		t.Fatal("claim reached the SDK before validation")
	}
}

func TestQueueClaimRejectsWaitLongerThanCommandTimeoutBeforeConnecting(t *testing.T) {
	t.Parallel()

	client := &queueClaimClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "--timeout", "10s", "queue", "claim", "email",
		"--worker", "mailer-1", "--wait", "30s",
	})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "--timeout") || !strings.Contains(err.Error(), "--wait") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.calls != 0 {
		t.Fatal("invalid wait/timeout combination reached the SDK")
	}
}

func TestQueueReclaimForwardsWorkerAndNewLease(t *testing.T) {
	t.Parallel()

	client := &queueReclaimClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "queue", "reclaim", "email", "--worker", "mailer-2",
		"--lease", "45s", "--limit", "3", "--partition", "tenant-a",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 || client.options.Type != "email" || client.options.Worker != "mailer-2" ||
		client.options.LeaseMS != int64((45*time.Second)/time.Millisecond) || client.options.Limit != 3 ||
		client.options.PartitionKey != "tenant-a" || !client.options.JobOnly {
		t.Fatalf("reclaim options = %#v", client.options)
	}
}

func TestQueueCompleteRequiresAndForwardsClaimTokens(t *testing.T) {
	t.Parallel()

	client := &queueCompleteClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "queue", "ack", "email-42", `{"sent":true}`, "--json",
		"--lease-token", "lease", "--fencing-token", "7",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.options.LeaseToken != "lease" || client.options.FencingToken != 7 || !client.options.ReturnRecord {
		t.Fatalf("complete options = %#v", client.options)
	}
	if !reflect.DeepEqual(client.options.Result, map[string]any{"sent": true}) {
		t.Fatalf("result = %#v", client.options.Result)
	}
}

func TestQueueCompleteRejectsMissingTokensBeforeConnecting(t *testing.T) {
	t.Parallel()

	client := &queueCompleteClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "queue", "complete", "email-42"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "lease token is required") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.calls != 0 {
		t.Fatal("complete reached the SDK before token validation")
	}
}

func TestQueueCancelAllowsUnclaimedFencingGenerationZero(t *testing.T) {
	t.Parallel()

	client := &queueCancelClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "queue", "cancel", "email-42", "obsolete"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 || client.options.FencingToken != 0 || client.options.LeaseToken != "" {
		t.Fatalf("cancel options = %#v", client.options)
	}
}

func TestFlowPolicyOptionsSupportRetryAndFIFO(t *testing.T) {
	t.Parallel()

	command := &cobra.Command{Use: "set"}
	command.Flags().String("max-active", "", "")
	command.Flags().Int("max-retries", 0, "")
	command.Flags().String("backoff", "", "")
	command.Flags().Duration("base-delay", 0, "")
	command.Flags().Duration("max-delay", 0, "")
	command.Flags().Int("jitter-percent", 0, "")
	command.Flags().String("exhausted-to", "", "")
	command.Flags().Bool("replace", false, "")
	command.Flags().Int64("expected-generation", 0, "")
	command.Flags().StringSlice("indexed-attribute", nil, "")
	command.Flags().Bool("clear-indexed-attributes", false, "")
	command.Flags().String("indexed-state-meta", "", "")
	command.Flags().StringArray("state-mode", nil, "")
	if err := command.Flags().Set("max-retries", "5"); err != nil {
		t.Fatal(err)
	}
	if err := command.Flags().Set("state-mode", "queued=FIFO"); err != nil {
		t.Fatal(err)
	}
	flags := flowPolicySetFlags{maxRetries: 5, stateModes: []string{"queued=FIFO"}}
	options, err := flags.options(command)
	if err != nil {
		t.Fatal(err)
	}
	if options.Retry == nil || options.Retry.MaxRetries != 5 || !options.Retry.MaxRetriesSet {
		t.Fatalf("retry policy = %#v", options.Retry)
	}
	if options.StatePolicies["queued"].Mode != ferricstore.FlowStateModeFIFO {
		t.Fatalf("state policies = %#v", options.StatePolicies)
	}
}

func TestFlowPolicySetRequiresConfirmationBeforeConnecting(t *testing.T) {
	t.Parallel()

	client := &queuePolicyClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "queue", "policy", "set", "email", "--max-retries", "5"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.calls != 0 {
		t.Fatal("policy update reached the SDK without confirmation")
	}
}

func TestQueueClaimDoesNotAdvertiseUnsupportedValueProjection(t *testing.T) {
	t.Parallel()

	queueClaim, _, err := newQueueCommand(dependencies{runtime: &runtimeOptions{timeout: time.Second}}).Find([]string{"claim"})
	if err != nil {
		t.Fatal(err)
	}
	if flag := queueClaim.Flags().Lookup("value"); flag != nil {
		t.Fatal("queue claim advertises --value even though the SDK claim result cannot return values")
	}

	workflowClaim, _, err := newWorkflowCommand(dependencies{runtime: &runtimeOptions{timeout: time.Second}}).Find([]string{"claim"})
	if err != nil {
		t.Fatal(err)
	}
	if flag := workflowClaim.Flags().Lookup("value"); flag == nil {
		t.Fatal("workflow claim lost supported --value projection")
	}
}

func TestFlowRecordOutputIncludesValueProjectionDiagnostics(t *testing.T) {
	t.Parallel()

	got := flowRecordOutput(&ferricstore.FlowRecord{
		ID:               "order-42",
		Type:             "order",
		State:            "running",
		IndexedStateMeta: "attempt",
		ValueSizes:       map[string]any{"receipt": int64(2048)},
		ValueOmitted:     map[string]any{"receipt": true},
		ValueMissing:     map[string]any{"invoice": true},
	}).(map[string]any)
	for _, key := range []string{"indexed_state_meta", "value_sizes", "value_omitted", "value_missing"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("flow output omitted %q: %#v", key, got)
		}
	}
}

func TestEveryQueueLeafHasDescriptionAndExample(t *testing.T) {
	t.Parallel()

	assertLeafHelp(t, newQueueCommand(dependencies{runtime: &runtimeOptions{timeout: time.Second}}))
}

func assertLeafHelp(t *testing.T, command *cobra.Command) {
	t.Helper()
	children := command.Commands()
	if len(children) == 0 {
		if strings.TrimSpace(command.Short) == "" {
			t.Errorf("%s has no short help", command.CommandPath())
		}
		if strings.TrimSpace(command.Example) == "" {
			t.Errorf("%s has no example", command.CommandPath())
		}
		return
	}
	for _, child := range children {
		if child.Name() == "help" || child.Name() == "completion" {
			continue
		}
		assertLeafHelp(t, child)
	}
}
