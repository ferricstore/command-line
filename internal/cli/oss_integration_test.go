//go:build integration

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ferricstore/command-line/internal/buildinfo"
	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/credential"
	"github.com/ferricstore/command-line/internal/ferric"
	"github.com/ferricstore/command-line/internal/profile"
)

func TestIntegrationOSSWorkflowQueryAndSchedule(t *testing.T) {
	if os.Getenv("FERRICSTORE_OSS_CLI_TEST") != "1" {
		t.Skip("OSS CLI integration is run by scripts/integration-login-oss.sh")
	}

	address := requiredCLIIntegrationEnv(t, "FERRICSTORE_OSS_ADDR")
	username := requiredCLIIntegrationEnv(t, "FERRICSTORE_OSS_USERNAME")
	password := requiredCLIIntegrationEnv(t, "FERRICSTORE_OSS_PASSWORD")
	profileName := "oss-integration"
	profiles := profile.NewFileStore(t.TempDir() + "/profiles.json")
	credentials := newIntegrationCredentialStore()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := profiles.Put(ctx, profile.Profile{
		Name: profileName,
		URL:  "ferric://" + address,
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: username,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put(ctx, profileName, password); err != nil {
		t.Fatal(err)
	}
	connections := connection.NewService(
		profiles,
		credentials,
		connection.NewPasswordProvider(ferric.PasswordClientFactory{}),
	)
	options := []Option{
		WithConnectionService(connections),
		WithProfileManager(profiles),
		WithCredentialStore(credentials),
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	flowType := "cli-query-" + suffix
	flowID := "cli-flow-" + suffix
	partition := "cli-partition-" + suffix
	scheduleID := "cli-schedule-" + suffix
	query := "FROM runs WHERE partition_key = @partition AND type = @type ORDER BY updated_at_ms ASC LIMIT 20 RETURN RECORDS"
	commonParams := []string{"--param", "partition=" + partition, "--param", "type=" + flowType}

	runIntegrationCLI(t, options,
		"workflow", "start", flowType, flowID, `{"source":"cli-integration"}`, "--json",
		"--partition", partition, "--attribute", "suite=command-line",
	)

	queryOutput := waitForIntegrationCLI(t, 20*time.Second, func() (string, error) {
		output, err := executeIntegrationCLI(options, append([]string{"workflow", "query", query}, commonParams...)...)
		if err != nil || !strings.Contains(output, flowID) {
			return "", errors.New("workflow is not visible through FQL yet")
		}
		return output, nil
	})
	assertIntegrationRecordID(t, queryOutput, flowID)

	explainArgs := append([]string{"workflow", "query", "explain", query}, commonParams...)
	explain := decodeIntegrationObject(t, runIntegrationCLI(t, options, explainArgs...))
	if status, _ := explain["status"].(string); status != "planned" {
		t.Fatalf("explain status = %#v", explain["status"])
	}
	if _, exposed := explain["raw"]; exposed {
		t.Fatal("explain output exposed SDK Raw data")
	}

	indexes := decodeIntegrationObject(t, runIntegrationCLI(t, options, "workflow", "query", "indexes"))
	if contract, _ := indexes["contract_version"].(string); contract != "ferric.flow.query.indexes/v1" {
		t.Fatalf("query index contract = %#v", indexes["contract_version"])
	}
	if _, exposed := indexes["raw"]; exposed {
		t.Fatal("query-index output exposed SDK Raw data")
	}

	listOutput := waitForIntegrationCLI(t, 20*time.Second, func() (string, error) {
		output, err := executeIntegrationCLI(options, "workflow", "list", flowType, "--partition", partition, "--state", "queued")
		if err != nil || !strings.Contains(output, flowID) {
			return "", errors.New("workflow is not visible through convenience list yet")
		}
		return output, nil
	})
	assertIntegrationListRecordID(t, listOutput, flowID)

	searchOutput := waitForIntegrationCLI(t, 20*time.Second, func() (string, error) {
		output, err := executeIntegrationCLI(
			options,
			"workflow", "search", "--type", flowType, "--partition", partition,
			"--attribute", "suite=command-line",
		)
		if err != nil || !strings.Contains(output, flowID) {
			return "", errors.New("workflow is not visible through convenience search yet")
		}
		return output, nil
	})
	assertIntegrationListRecordID(t, searchOutput, flowID)

	startAt := strconv.FormatInt(time.Now().Add(time.Hour).UnixMilli(), 10)
	created := decodeIntegrationObject(t, runIntegrationCLI(
		t,
		options,
		"workflow", "schedule", "create", scheduleID,
		"--type", flowType, "--id-prefix", flowID+"-scheduled", "--partition", partition,
		"--every", "1h", "--start-at", startAt, "--catchup", "fire_once", "--overlap", "skip",
	))
	if created["kind"] != "interval" || created["catchup_policy"] != "fire_once" {
		t.Fatalf("created schedule = %#v", created)
	}
	defer func() {
		if _, err := executeIntegrationCLI(options, "workflow", "schedule", "delete", scheduleID, "--yes"); err != nil {
			t.Errorf("delete integration schedule: %v", err)
		}
	}()

	described := decodeIntegrationObject(t, runIntegrationCLI(t, options, "workflow", "schedule", "describe", scheduleID))
	if described["kind"] != "interval" || described["catchup_policy"] != "fire_once" {
		t.Fatalf("described schedule = %#v", described)
	}
	if every, _ := described["every_ms"].(float64); every != float64(time.Hour.Milliseconds()) {
		t.Fatalf("schedule every_ms = %#v", described["every_ms"])
	}
}

func executeIntegrationCLI(options []Option, args ...string) (string, error) {
	command := New(buildinfo.Info{}, options...)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(append([]string{"--profile", "oss-integration", "--output", "json"}, args...))
	err := command.Execute()
	return output.String(), err
}

func runIntegrationCLI(t *testing.T, options []Option, args ...string) string {
	t.Helper()
	output, err := executeIntegrationCLI(options, args...)
	if err != nil {
		t.Fatalf("ferric %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return output
}

func waitForIntegrationCLI(t *testing.T, timeout time.Duration, operation func() (string, error)) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		output, err := operation()
		if err == nil {
			return output
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for CLI result: %v", lastErr)
	return ""
}

func decodeIntegrationObject(t *testing.T, output string) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal([]byte(output), &value); err != nil {
		t.Fatalf("decode CLI output %q: %v", output, err)
	}
	return value
}

func assertIntegrationRecordID(t *testing.T, output, want string) {
	t.Helper()
	value := decodeIntegrationObject(t, output)
	records, ok := value["records"].([]any)
	if !ok || len(records) == 0 {
		t.Fatalf("query records = %#v", value["records"])
	}
	record, ok := records[0].(map[string]any)
	if !ok || record["id"] != want {
		t.Fatalf("query record = %#v, want ID %q", records[0], want)
	}
}

func assertIntegrationListRecordID(t *testing.T, output, want string) {
	t.Helper()
	var records []map[string]any
	if err := json.Unmarshal([]byte(output), &records); err != nil {
		t.Fatalf("decode CLI list output %q: %v", output, err)
	}
	for _, record := range records {
		if record["id"] == want {
			return
		}
	}
	t.Fatalf("CLI list output does not contain %q: %#v", want, records)
}

func requiredCLIIntegrationEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

type integrationCredentialStore struct {
	mu     sync.Mutex
	values map[string]string
}

func newIntegrationCredentialStore() *integrationCredentialStore {
	return &integrationCredentialStore{values: make(map[string]string)}
}

func (store *integrationCredentialStore) Put(ctx context.Context, name, secret string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.values[name] = secret
	return nil
}

func (store *integrationCredentialStore) Get(ctx context.Context, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	value, ok := store.values[name]
	if !ok {
		return "", credential.ErrNotFound
	}
	return value, nil
}

func (store *integrationCredentialStore) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.values[name]; !ok {
		return credential.ErrNotFound
	}
	delete(store.values, name)
	return nil
}

var _ credential.Store = (*integrationCredentialStore)(nil)
