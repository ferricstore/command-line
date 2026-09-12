package outputcontract

import (
	"reflect"
	"testing"

	ferricstore "github.com/ferricstore/ferricstore-go"
)

func TestApprovalHasExplicitCompactSchema(t *testing.T) {
	t.Parallel()

	got := Approval(&ferricstore.ApprovalResult{
		ID:            "approval-42",
		Status:        "pending",
		RequestedAtMS: 10,
		Raw:           map[string]any{"transport_only": true},
	})
	want := map[string]any{
		"id":              "approval-42",
		"status":          "pending",
		"requested_at_ms": int64(10),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Approval() = %#v, want %#v", got, want)
	}
}

func TestFlowExplainPreservesContractZerosAndOmitsRaw(t *testing.T) {
	t.Parallel()

	got := FlowExplain(&ferricstore.FlowExplainResult{
		Version: "ferric.flow.explain/v1",
		Actual:  &ferricstore.FlowQueryUsage{ScannedEntries: 3},
		Raw:     map[string]any{"transport_only": true},
	}).(map[string]any)
	if got["version"] != "ferric.flow.explain/v1" || got["status"] != "" {
		t.Fatalf("FlowExplain() contract fields = %#v", got)
	}
	actual, ok := got["actual"].(map[string]any)
	if !ok || actual["scanned_entries"] != int64(3) || actual["range_seeks"] != int64(0) {
		t.Fatalf("FlowExplain() actual = %#v", got["actual"])
	}
	if _, exposed := got["raw"]; exposed {
		t.Fatal("FlowExplain() exposed Raw")
	}
}

func TestFlowQueryIndexesHasExplicitNestedSchema(t *testing.T) {
	t.Parallel()

	got := FlowQueryIndexes(&ferricstore.FlowQueryIndexStatus{
		ContractVersion: "ferric.flow.query.indexes/v1",
		Registry:        ferricstore.FlowQueryIndexRegistry{CatalogVersion: 2},
		Indexes: []ferricstore.FlowQueryIndex{{
			ID:        "by_partition",
			Version:   1,
			Queryable: true,
			Fields: []ferricstore.FlowQueryIndexField{{
				Name: "partition_key", Direction: "asc", Encoding: "ordered",
				Raw: map[string]any{"transport_only": true},
			}},
			Raw: map[string]any{"transport_only": true},
		}},
		Raw: map[string]any{"transport_only": true},
	}).(map[string]any)
	indexes := got["indexes"].([]any)
	index := indexes[0].(map[string]any)
	if index["id"] != "by_partition" || index["queryable"] != true {
		t.Fatalf("FlowQueryIndexes() index = %#v", index)
	}
	if _, exposed := index["raw"]; exposed {
		t.Fatal("FlowQueryIndexes() exposed Raw")
	}
}

func TestScheduleFireDueHasExplicitCompactSchema(t *testing.T) {
	t.Parallel()

	got := ScheduleFireDue(ferricstore.ScheduleFireDueResult{
		Claimed: 2,
		Fired:   1,
		Errors:  []ferricstore.ScheduleFireDueError{{ID: "daily", Reason: "target_failed"}},
		Raw:     map[string]any{"transport_only": true},
	})
	want := map[string]any{
		"claimed":   int64(2),
		"fired":     int64(1),
		"skipped":   int64(0),
		"coalesced": int64(0),
		"errors":    []any{map[string]any{"id": "daily", "reason": "target_failed"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScheduleFireDue() = %#v, want %#v", got, want)
	}
}

func TestScheduleFireDuePreservesZeroCounters(t *testing.T) {
	t.Parallel()

	got := ScheduleFireDue(ferricstore.ScheduleFireDueResult{}).(map[string]any)
	for _, key := range []string{"claimed", "fired", "skipped", "coalesced"} {
		if value, ok := got[key]; !ok || value != int64(0) {
			t.Fatalf("ScheduleFireDue() %s = %#v, present=%t", key, value, ok)
		}
	}
}

func TestBudgetPreservesFalseAndZeroCapacityFields(t *testing.T) {
	t.Parallel()

	got := Budget(&ferricstore.BudgetResult{Scope: "payments"}).(map[string]any)
	if value, ok := got["over_budget"]; !ok || value != false {
		t.Fatalf("Budget() over_budget = %#v, present=%t", value, ok)
	}
	for _, key := range []string{"limit", "used", "remaining", "reservations_count"} {
		if value, ok := got[key]; !ok || value != int64(0) {
			t.Fatalf("Budget() %s = %#v, present=%t", key, value, ok)
		}
	}
}

func TestLimitAndCircuitPreserveZeroCounters(t *testing.T) {
	t.Parallel()

	limit := Limit(&ferricstore.LimitResult{Scope: "workers"}).(map[string]any)
	for _, key := range []string{"limit", "free", "epoch", "config_version"} {
		if value, ok := limit[key]; !ok || value != int64(0) {
			t.Fatalf("Limit() %s = %#v, present=%t", key, value, ok)
		}
	}
	circuit := Circuit(&ferricstore.CircuitBreakerStatus{Scope: "payments"}).(map[string]any)
	for _, key := range []string{"failures", "failure_count", "half_open_in_flight", "half_open_successes", "event_count", "retry_after_ms"} {
		if value, ok := circuit[key]; !ok || value != int64(0) {
			t.Fatalf("Circuit() %s = %#v, present=%t", key, value, ok)
		}
	}
}

func TestPolicyHasExplicitSchemaAndOmitsRaw(t *testing.T) {
	t.Parallel()

	got := Policy(ferricstore.PolicySnapshot{
		Type:       "email",
		Generation: 3,
		States: map[string]ferricstore.PolicyStateSnapshot{
			"queued": {Mode: ferricstore.FlowStateModeFIFO, Raw: map[string]any{"transport_only": true}},
		},
		Raw: map[string]any{"transport_only": true},
	}).(map[string]any)
	if got["type"] != "email" || got["generation"] != int64(3) {
		t.Fatalf("Policy() = %#v", got)
	}
	if _, exposed := got["raw"]; exposed {
		t.Fatal("Policy() exposed Raw")
	}
	queued := got["states"].(map[string]any)["queued"].(map[string]any)
	if _, exposed := queued["raw"]; exposed {
		t.Fatal("Policy() exposed nested Raw")
	}
}
