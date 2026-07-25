package cli

import (
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
func (client *scheduleTestClient) ScheduleCreate(_ context.Context, id string, options ferricstore.ScheduleOptions) (ferricstore.ScheduleResult, error) {
	client.createdID = id
	client.createOptions = options
	return ferricstore.ScheduleResult{ID: id, Kind: options.Kind, Status: "active", Target: options.Target}, nil
}
func (client *scheduleTestClient) ScheduleDelete(_ context.Context, id string, _ *int64) (ferricstore.ScheduleResult, error) {
	client.deleteCalls++
	client.deletedID = id
	return ferricstore.ScheduleResult{ID: id, Status: "deleted"}, nil
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
	command.SetArgs([]string{"--profile", "production", "workflow", "schedule", "delete", "daily-report", "--yes"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.deleteCalls != 1 || client.deletedID != "daily-report" {
		t.Fatalf("delete calls/id = %d/%q", client.deleteCalls, client.deletedID)
	}
}

func TestEveryScheduleLeafHasDescriptionAndExample(t *testing.T) {
	t.Parallel()

	assertLeafHelp(t, newScheduleCommand(dependencies{runtime: &runtimeOptions{timeout: time.Second}}))
}
