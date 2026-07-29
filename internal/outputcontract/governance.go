// Package outputcontract defines the stable, user-visible representation of
// typed FerricStore SDK responses.
package outputcontract

import (
	"strconv"

	ferricstore "github.com/ferricstore/ferricstore-go"
)

// GovernanceOverview converts the SDK governance summary without exposing its
// transport Raw snapshot or future SDK fields.
func GovernanceOverview(value ferricstore.GovernanceOverview) any {
	output := map[string]any{}
	putMap(output, "counts", value.Counts)
	if value.Approvals != nil {
		output["approvals"] = approvalList(value.Approvals)
	}
	if value.Budgets != nil {
		output["budgets"] = budgetList(value.Budgets)
	}
	if value.Limits != nil {
		output["limits"] = limitList(value.Limits)
	}
	if value.Circuits != nil {
		output["circuits"] = circuitList(value.Circuits)
	}
	if value.Effects != nil {
		output["effects"] = effectList(value.Effects)
	}
	return output
}

// Approval converts one SDK approval response.
func Approval(value *ferricstore.ApprovalResult) any {
	if value == nil {
		return nil
	}
	return approval(*value)
}

// ApprovalList converts a bounded SDK approval collection.
func ApprovalList(values []ferricstore.ApprovalResult) any {
	if values == nil {
		return nil
	}
	return approvalList(values)
}

func approvalList(values []ferricstore.ApprovalResult) []any {
	output := make([]any, len(values))
	for index := range values {
		output[index] = approval(values[index])
	}
	return output
}

func approval(value ferricstore.ApprovalResult) map[string]any {
	output := map[string]any{}
	putString(output, "id", value.ID)
	putString(output, "flow_id", value.FlowID)
	putString(output, "scope", value.Scope)
	putString(output, "status", value.Status)
	putString(output, "reason", value.Reason)
	putString(output, "requested_by", value.RequestedBy)
	putString(output, "approver", value.Approver)
	putString(output, "decision_reason", value.DecisionReason)
	putStrings(output, "assignees", value.Assignees)
	putString(output, "policy_hash", value.PolicyHash)
	putString(output, "policy_version", value.PolicyVersion)
	putInt64(output, "requested_at_ms", value.RequestedAtMS)
	putInt64(output, "decided_at_ms", value.DecidedAtMS)
	putInt64(output, "expires_at_ms", value.ExpiresAtMS)
	return output
}

// Budget converts one SDK budget response.
func Budget(value *ferricstore.BudgetResult) any {
	if value == nil {
		return nil
	}
	return budget(*value)
}

// BudgetList converts a bounded SDK budget collection.
func BudgetList(values []ferricstore.BudgetResult) any {
	if values == nil {
		return nil
	}
	return budgetList(values)
}

func budgetList(values []ferricstore.BudgetResult) []any {
	output := make([]any, len(values))
	for index := range values {
		output[index] = budget(values[index])
	}
	return output
}

func budget(value ferricstore.BudgetResult) map[string]any {
	output := map[string]any{}
	putString(output, "scope", value.Scope)
	putString(output, "status", value.Status)
	putInt64(output, "limit", value.Limit)
	putInt64(output, "window_ms", value.WindowMS)
	putInt64(output, "window_start_ms", value.WindowStartMS)
	putInt64(output, "used", value.Used)
	putInt64(output, "remaining", value.Remaining)
	putBool(output, "over_budget", value.OverBudget)
	putInt64(output, "reservations_count", value.ReservationsCount)
	putString(output, "reservation_id", value.ReservationID)
	putInt64(output, "reserved_amount", value.ReservedAmount)
	putInt64(output, "actual_amount", value.ActualAmount)
	putMap(output, "usage", value.Usage)
	putInt64(output, "overage_amount", value.OverageAmount)
	putInt64(output, "reserved_at_ms", value.ReservedAtMS)
	putInt64(output, "settled_at_ms", value.SettledAtMS)
	return output
}

// Limit converts one SDK distributed-limit response.
func Limit(value *ferricstore.LimitResult) any {
	if value == nil {
		return nil
	}
	return limit(*value)
}

// LimitList converts a bounded SDK distributed-limit collection.
func LimitList(values []ferricstore.LimitResult) any {
	if values == nil {
		return nil
	}
	return limitList(values)
}

func limitList(values []ferricstore.LimitResult) []any {
	output := make([]any, len(values))
	for index := range values {
		output[index] = limit(values[index])
	}
	return output
}

func limit(value ferricstore.LimitResult) map[string]any {
	output := map[string]any{}
	putString(output, "scope", value.Scope)
	putInt64(output, "limit", value.Limit)
	putInt64(output, "free", value.Free)
	putInt64(output, "epoch", value.Epoch)
	putInt64(output, "config_version", value.ConfigVersion)
	putString(output, "policy_version_hash", value.PolicyVersionHash)
	if value.Leases != nil {
		leases := make(map[string]any, len(value.Leases))
		for shard, lease := range value.Leases {
			leases[strconv.FormatInt(shard, 10)] = limitLease(lease)
		}
		output["leases"] = leases
	}
	if value.Lease != nil {
		output["lease"] = limitLease(*value.Lease)
	}
	putStrings(output, "reservation_ids", value.ReservationIDs)
	return output
}

func limitLease(value ferricstore.LimitLeaseState) map[string]any {
	output := map[string]any{}
	putInt64(output, "shard_id", value.ShardID)
	putInt64(output, "epoch", value.Epoch)
	putInt64(output, "expires_at_ms", value.ExpiresAtMS)
	putInt64(output, "available", value.Available)
	putInt64(output, "in_use", value.InUse)
	putInt64(output, "pending_reclaim", value.PendingReclaim)
	putFloat64(output, "drain_rate", value.DrainRate)
	putInt64(output, "last_spend_at_ms", value.LastSpendAtMS)
	return output
}

// Circuit converts one SDK circuit-breaker response.
func Circuit(value *ferricstore.CircuitBreakerStatus) any {
	if value == nil {
		return nil
	}
	return circuit(*value)
}

func circuitList(values []ferricstore.CircuitBreakerStatus) []any {
	output := make([]any, len(values))
	for index := range values {
		output[index] = circuit(values[index])
	}
	return output
}

func circuit(value ferricstore.CircuitBreakerStatus) map[string]any {
	output := map[string]any{}
	putString(output, "scope", value.Scope)
	putString(output, "status", value.Status)
	putInt64(output, "failure_threshold", value.FailureThreshold)
	putInt64(output, "open_ms", value.OpenMS)
	putInt64(output, "opened_at_ms", value.OpenedAtMS)
	putInt64(output, "failures", value.Failures)
	putInt64(output, "failure_count", value.FailureCount)
	putInt64(output, "window_ms", value.WindowMS)
	putInt64(output, "min_calls", value.MinCalls)
	putInt64(output, "failure_rate_pct", value.FailureRatePct)
	putInt64(output, "latency_threshold_ms", value.LatencyThresholdMS)
	putStrings(output, "error_classes", value.ErrorClasses)
	putInt64(output, "half_open_max_probes", value.HalfOpenMaxProbes)
	putInt64(output, "half_open_success_threshold", value.HalfOpenSuccessThreshold)
	putInt64(output, "half_open_in_flight", value.HalfOpenInFlight)
	putInt64(output, "half_open_successes", value.HalfOpenSuccesses)
	putInt64(output, "half_open_started_at_ms", value.HalfOpenStartedAtMS)
	putInt64(output, "last_failure_ms", value.LastFailureMS)
	putInt64(output, "last_success_ms", value.LastSuccessMS)
	putInt64(output, "updated_at_ms", value.UpdatedAtMS)
	if value.Events != nil {
		output["events"] = value.Events
	}
	putInt64(output, "event_count", value.EventCount)
	putInt64(output, "retry_after_ms", value.RetryAfterMS)
	return output
}

// Effect converts one SDK governed-effect response.
func Effect(value *ferricstore.EffectResult) any {
	if value == nil {
		return nil
	}
	return effect(*value)
}

func effectList(values []ferricstore.EffectResult) []any {
	output := make([]any, len(values))
	for index := range values {
		output[index] = effect(values[index])
	}
	return output
}

func effect(value ferricstore.EffectResult) map[string]any {
	output := map[string]any{}
	putString(output, "id", value.ID)
	putString(output, "flow_id", value.FlowID)
	putString(output, "partition_key", value.PartitionKey)
	putString(output, "flow_type", value.FlowType)
	putString(output, "state", value.State)
	putString(output, "effect_key", value.EffectKey)
	putString(output, "effect_type", value.EffectType)
	putString(output, "status", value.Status)
	putString(output, "decision", value.Decision)
	putString(output, "scope", value.Scope)
	putString(output, "external_id", value.ExternalID)
	putString(output, "error", value.Error)
	putString(output, "reason", value.Reason)
	putString(output, "operation_digest", value.OperationDigest)
	putString(output, "idempotency_key", value.IdempotencyKey)
	putString(output, "policy_hash", value.PolicyHash)
	putString(output, "policy_version", value.PolicyVersion)
	putInt64(output, "latency_ms", value.LatencyMS)
	putInt64(output, "created_at_ms", value.CreatedAtMS)
	putInt64(output, "updated_at_ms", value.UpdatedAtMS)
	putInt64(output, "reserved_at_ms", value.ReservedAtMS)
	putInt64(output, "confirmed_at_ms", value.ConfirmedAtMS)
	putInt64(output, "failed_at_ms", value.FailedAtMS)
	putInt64(output, "compensated_at_ms", value.CompensatedAtMS)
	return output
}

func putString(output map[string]any, name, value string) {
	if value != "" {
		output[name] = value
	}
}

func putInt64(output map[string]any, name string, value int64) {
	if value != 0 {
		output[name] = value
	}
}

func putFloat64(output map[string]any, name string, value float64) {
	if value != 0 {
		output[name] = value
	}
}

func putBool(output map[string]any, name string, value bool) {
	if value {
		output[name] = value
	}
}

func putStrings(output map[string]any, name string, value []string) {
	if value != nil {
		output[name] = value
	}
}

func putMap(output map[string]any, name string, value map[string]any) {
	if value != nil {
		output[name] = value
	}
}
