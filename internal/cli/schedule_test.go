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

type scheduleTestClient struct {
	createdID     string
	createOptions ferricstore.ScheduleOptions
	deletedID     string
	deleteCalls   int
}

func (*scheduleTestClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (*scheduleTestClient) Close() error                                    { return nil }
func (client *scheduleTestClient) ScheduleCreate(_ context.Context, id string, options ferricstore.ScheduleOptions) (ferricstore.ScheduleRecord, error) {
	client.createdID = id
	client.createOptions = options
	return ferricstore.ScheduleRecord{ID: id, Kind: options.Kind, State: "active", Target: options.Target}, nil
}
func (client *scheduleTestClient) ScheduleDelete(_ context.Context, id string, _ ferricstore.ScheduleStatusOptions) error {
	client.deleteCalls++
	client.deletedID = id
	return nil
}

func TestScheduleCreateBuildsRecurringTarget(t *testing.T) {
	t.Parallel()

	client := &scheduleTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "schedule", "create", "daily-report", `{"scope":"daily"}`, "--json",
		"--type", "report", "--id-prefix", "daily-report", "--cron", "0 8 * * *", "--timezone", "UTC",
		"--overlap", "skip", "--partition", "tenant-a", "--value", "format=pdf", "--overwrite",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	options := client.createOptions
	if client.createdID != "daily-report" || options.Kind != "cron" || options.Cron != "0 8 * * *" || options.Timezone != "UTC" || options.OverlapPolicy != "skip" {
		t.Fatalf("schedule options = %#v", options)
	}
	wantPayload := map[string]any{"scope": "daily"}
	if !reflect.DeepEqual(options.Target["payload"], wantPayload) {
		t.Fatalf("target payload = %#v", options.Target["payload"])
	}
	if options.Target["type"] != "report" || options.Target["id_prefix"] != "daily-report" || options.Target["partition_key"] != "tenant-a" {
		t.Fatalf("target = %#v", options.Target)
	}
	if !reflect.DeepEqual(options.Target["values"], map[string]any{"format": "pdf"}) {
		t.Fatalf("target values = %#v", options.Target["values"])
	}
	if options.Overwrite == nil || !*options.Overwrite {
		t.Fatalf("overwrite = %#v", options.Overwrite)
	}
}

func TestScheduleStateCompletionMatchesSDKStates(t *testing.T) {
	t.Parallel()

	got, directive := completeScheduleState(nil, nil, "")
	want := []string{"active", "paused", "running", "completed", "failed", "cancelled", "all"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("completeScheduleState() = %#v, want %#v", got, want)
	}
	if directive == 0 {
		t.Fatal("completion should disable file suggestions")
	}
}

func TestScheduleFireDueRejectsWaitLongerThanCommandTimeoutBeforeConnecting(t *testing.T) {
	t.Parallel()

	client := &cliConnectionClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "--timeout", "10s", "workflow", "schedule", "fire-due",
		"--worker", "scheduler-1", "--wait", "30s",
	})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "--timeout") || !strings.Contains(err.Error(), "--wait") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.closed {
		t.Fatal("invalid wait/timeout combination opened a connection")
	}
}

func TestScheduleCreateRejectsMultipleTimingModesBeforeConnecting(t *testing.T) {
	t.Parallel()

	client := &scheduleTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "schedule", "create", "invalid", "--type", "report", "--after", "5m", "--every", "1h",
	})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "only one") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.createdID != "" {
		t.Fatal("invalid schedule reached the SDK")
	}
}

func TestScheduleCreateForwardsIntervalCatchupPolicy(t *testing.T) {
	t.Parallel()

	client := &scheduleTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "schedule", "create", "hourly-report",
		"--type", "report", "--id-prefix", "hourly", "--every", "1h", "--catchup", "fire_once",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.createOptions.CatchupPolicy != "fire_once" {
		t.Fatalf("catchup policy = %q", client.createOptions.CatchupPolicy)
	}
}

func TestScheduleDeleteRequiresExplicitConfirmation(t *testing.T) {
	t.Parallel()

	client := &scheduleTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "workflow", "schedule", "delete", "daily-report"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.deleteCalls != 0 {
		t.Fatal("schedule was deleted without confirmation")
	}
}

func TestScheduleDeleteForwardsConfirmedRequest(t *testing.T) {
	t.Parallel()

	client := &scheduleTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"--profile", "production", "workflow", "schedule", "delete", "daily-report", "--yes"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.deleteCalls != 1 || client.deletedID != "daily-report" {
		t.Fatalf("delete calls/id = %d/%q", client.deleteCalls, client.deletedID)
	}
	if !strings.Contains(output.String(), `"deleted": true`) || !strings.Contains(output.String(), `"id": "daily-report"`) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestScheduleResultOutputIncludesRichRecurrenceState(t *testing.T) {
	t.Parallel()

	every := int64(3_600_000)
	next := int64(2_000)
	catchup := int64(1_500)
	overlap := int64(1_800)
	result := scheduleResultOutput(&ferricstore.ScheduleRecord{
		ID:                  "hourly-report",
		Kind:                "interval",
		State:               "active",
		Target:              map[string]any{"type": "report"},
		CreatedAtMS:         1_000,
		EveryMS:             &every,
		CatchupPolicy:       "fire_once",
		CoalescedCount:      4,
		LastCatchupAtMS:     &catchup,
		LastCoalescedCount:  2,
		OverlapPolicy:       "skip",
		NextRunAtMS:         &next,
		FireCount:           3,
		Attempts:            5,
		LastOverlapAtMS:     &overlap,
		LastOverlapTargetID: "hourly-report-2",
		LastOverlapReason:   "target_running",
		SkippedCount:        1,
		LastTargetID:        "hourly-report-3",
		Raw:                 map[string]any{"transport_only": true},
	}).(map[string]any)

	for name, want := range map[string]any{
		"every_ms":               every,
		"catchup_policy":         "fire_once",
		"coalesced_count":        int64(4),
		"last_catchup_at_ms":     catchup,
		"last_coalesced_count":   int64(2),
		"next_run_at_ms":         next,
		"last_overlap_at_ms":     overlap,
		"last_overlap_target_id": "hourly-report-2",
		"last_overlap_reason":    "target_running",
		"fire_count":             int64(3),
		"attempts":               int64(5),
		"skipped_count":          int64(1),
		"last_target_id":         "hourly-report-3",
	} {
		if got := result[name]; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s = %#v, want %#v", name, got, want)
		}
	}
	if _, exposed := result["raw"]; exposed {
		t.Fatal("schedule output exposes SDK Raw data")
	}
}

func TestEveryScheduleLeafHasDescriptionAndExample(t *testing.T) {
	t.Parallel()

	assertLeafHelp(t, newScheduleCommand(dependencies{runtime: &runtimeOptions{timeout: time.Second}}))
}
