package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/buildinfo"
	ferricstore "github.com/ferricstore/ferricstore-go"
	"github.com/spf13/cobra"
)

type queryTestClient struct {
	query        string
	params       map[string]any
	queryCalls   int
	explainCalls int
	analyzeCalls int
	indexIDs     []string
	queryError   error
}

func (*queryTestClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (*queryTestClient) Close() error                                    { return nil }
func (client *queryTestClient) FlowQuery(_ context.Context, query string, params map[string]any) (*ferricstore.FlowQueryResult, error) {
	client.queryCalls++
	client.query = query
	client.params = params
	if client.queryError != nil {
		return nil, client.queryError
	}
	return &ferricstore.FlowQueryResult{
		Version: "ferric.flow.query.result/v1",
		Records: []map[string]any{{"id": "order-42", "attempts": int64(3)}},
		Page:    &ferricstore.FlowQueryPage{},
		Quality: ferricstore.FlowQueryQuality{Exactness: "exact", Freshness: "current", Coverage: "complete", Pagination: "complete"},
		Usage:   ferricstore.FlowQueryUsage{ScannedEntries: 1, HydratedRecords: 1, ResultRecords: 1},
		Raw:     map[string]any{"transport_only": true},
	}, nil
}
func (client *queryTestClient) FlowExplain(_ context.Context, query string, params map[string]any) (*ferricstore.FlowExplainResult, error) {
	client.explainCalls++
	client.query = query
	client.params = params
	return queryExplainFixture(), nil
}
func (client *queryTestClient) FlowExplainAnalyze(_ context.Context, query string, params map[string]any) (*ferricstore.FlowExplainResult, error) {
	client.analyzeCalls++
	client.query = query
	client.params = params
	return queryExplainFixture(), nil
}
func (client *queryTestClient) FlowQueryIndexes(_ context.Context, indexIDs ...string) (*ferricstore.FlowQueryIndexStatus, error) {
	client.indexIDs = append([]string(nil), indexIDs...)
	return &ferricstore.FlowQueryIndexStatus{
		ContractVersion: "ferric.flow.query.indexes/v1",
		Registry:        ferricstore.FlowQueryIndexRegistry{CatalogVersion: 1},
		Indexes:         []ferricstore.FlowQueryIndex{},
		Raw:             map[string]any{"transport_only": true},
	}, nil
}

func queryExplainFixture() *ferricstore.FlowExplainResult {
	return &ferricstore.FlowExplainResult{
		Version:          "ferric.flow.explain/v1",
		QueryFingerprint: strings.Repeat("a", 64),
		Status:           "executed",
		Plan:             map[string]any{"index": "by_partition"},
		Actual:           &ferricstore.FlowQueryUsage{ScannedEntries: 1},
		Raw:              map[string]any{"transport_only": true},
	}
}

func TestWorkflowQueryForwardsTypedParametersAndPrintsContract(t *testing.T) {
	t.Parallel()

	client := &queryTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	query := "FROM runs WHERE partition_key = @partition AND attempts >= @minimum RETURN RECORDS"
	command.SetArgs([]string{
		"--profile", "production", "workflow", "query", query,
		"--param", "partition=tenant-a", "--params-json", `{"minimum":3,"terminal":false}`,
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.query != query || client.queryCalls != 1 {
		t.Fatalf("query/calls = %q/%d", client.query, client.queryCalls)
	}
	wantParams := map[string]any{"partition": "tenant-a", "minimum": int64(3), "terminal": false}
	if !reflect.DeepEqual(client.params, wantParams) {
		t.Fatalf("params = %#v, want %#v", client.params, wantParams)
	}
	if !strings.Contains(output.String(), `"records"`) || !strings.Contains(output.String(), `"usage"`) {
		t.Fatalf("output = %q", output.String())
	}
	if strings.Contains(output.String(), "transport_only") || strings.Contains(output.String(), `"raw"`) {
		t.Fatalf("output exposes SDK transport fields: %q", output.String())
	}
}

func TestWorkflowQueryReadsQueryAndParametersFromFiles(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	queryPath := filepath.Join(directory, "query.fql")
	paramsPath := filepath.Join(directory, "params.json")
	if err := os.WriteFile(queryPath, []byte("FROM runs WHERE partition_key = @partition RETURN COUNT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paramsPath, []byte(`{"partition":"tenant-a"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &queryTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{
		"--profile", "production", "workflow", "query", "--file", queryPath, "--params-file", paramsPath,
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.query != "FROM runs WHERE partition_key = @partition RETURN COUNT\n" {
		t.Fatalf("query = %q", client.query)
	}
	if !reflect.DeepEqual(client.params, map[string]any{"partition": "tenant-a"}) {
		t.Fatalf("params = %#v", client.params)
	}
}

func TestWorkflowQueryExplainAnalyzeUsesAnalyzeAPI(t *testing.T) {
	t.Parallel()

	client := &queryTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"--profile", "production", "workflow", "query", "explain",
		"FROM runs WHERE partition_key = @partition RETURN COUNT", "--param", "partition=tenant-a", "--analyze",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.analyzeCalls != 1 || client.explainCalls != 0 {
		t.Fatalf("analyze/explain calls = %d/%d", client.analyzeCalls, client.explainCalls)
	}
	if !strings.Contains(output.String(), `"scanned_entries": 1`) || strings.Contains(output.String(), "transport_only") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestWorkflowQueryIndexesForwardsOptionalID(t *testing.T) {
	t.Parallel()

	client := &queryTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "workflow", "query", "indexes", "by_partition"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(client.indexIDs, []string{"by_partition"}) {
		t.Fatalf("index IDs = %#v", client.indexIDs)
	}
}

func TestWorkflowQueryRejectsInvalidParametersBeforeSDKCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "duplicate JSON key", args: []string{"workflow", "query", "RETURN COUNT", "--params-json", `{"tenant":"a","tenant":"b"}`}, want: "duplicate query parameter"},
		{name: "object value", args: []string{"workflow", "query", "RETURN COUNT", "--params-json", `{"tenant":{"id":"a"}}`}, want: "string, boolean, or number"},
		{name: "duplicate merge", args: []string{"workflow", "query", "RETURN COUNT", "--params-json", `{"tenant":"a"}`, "--param", "tenant=b"}, want: "duplicate query parameter"},
		{name: "stdin collision", args: []string{"workflow", "query", "--file", "-", "--params-file", "-"}, want: "cannot both read from stdin"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &queryTestClient{}
			command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
			command.SetArgs(append([]string{"--profile", "production"}, test.args...))
			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute() error = %v, want %q", err, test.want)
			}
			if client.queryCalls != 0 {
				t.Fatal("invalid query reached the SDK")
			}
		})
	}
}

func TestFlowQueryInputEnforcesSDKBounds(t *testing.T) {
	t.Parallel()

	if err := validateFlowQueryInput(strings.Repeat("q", flowQueryMaxBytes+1)); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized query error = %v", err)
	}
	if _, err := decodeFlowQueryParams([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("invalid UTF-8 parameters error = %v", err)
	}
	oversizedValue := `{"value":"` + strings.Repeat("v", flowQueryMaxParameterValue+1) + `"}`
	if _, err := decodeFlowQueryParams([]byte(oversizedValue)); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized parameter error = %v", err)
	}

	flags := flowQueryInputFlags{params: make([]string, flowQueryMaxParameters+1)}
	for index := range flags.params {
		flags.params[index] = fmt.Sprintf("parameter_%d=value", index)
	}
	if _, err := flags.readParams(&cobra.Command{}); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("too many parameters error = %v", err)
	}
}

func TestWorkflowQueryRejectsInvalidIndexIDBeforeSDKCall(t *testing.T) {
	t.Parallel()

	client := &queryTestClient{}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "workflow", "query", "indexes", "invalid/index"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "query index ID") {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.indexIDs != nil {
		t.Fatal("invalid index ID reached the SDK")
	}
}

func TestWorkflowQueryFormatsServerDiagnostic(t *testing.T) {
	t.Parallel()

	client := &queryTestClient{queryError: &ferricstore.FlowQueryError{
		Code:     "FQL_UNKNOWN_FIELD",
		Message:  "unknown field",
		Detail:   "field customer_tier is not indexed",
		Hint:     "use attributes.customer_tier",
		Position: &ferricstore.FlowQueryErrorPosition{Byte: 18, Line: 1, Column: 19},
	}}
	command := New(buildinfo.Info{}, WithConnectionService(newCLIConnectionService(client)))
	command.SetArgs([]string{"--profile", "production", "workflow", "query", "FROM runs RETURN COUNT"})

	err := command.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil")
	}
	for _, text := range []string{"FQL_UNKNOWN_FIELD", "line 1, column 19", "field customer_tier", "use attributes.customer_tier"} {
		if !strings.Contains(err.Error(), text) {
			t.Fatalf("error = %q, missing %q", err, text)
		}
	}
	var queryErr *ferricstore.FlowQueryError
	if !errors.As(err, &queryErr) {
		t.Fatalf("formatted error no longer unwraps to FlowQueryError: %v", err)
	}
	if count := strings.Count(err.Error(), "unknown field"); count != 1 {
		t.Fatalf("formatted error repeats the server message %d times: %q", count, err)
	}
}
