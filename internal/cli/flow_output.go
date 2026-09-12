package cli

import ferricstore "github.com/ferricstore/ferricstore-go"

func flowRecordOutput(record *ferricstore.FlowRecord) any {
	if record == nil {
		return nil
	}
	result := map[string]any{
		"id":      record.ID,
		"type":    record.Type,
		"state":   record.State,
		"version": record.Version,
	}
	putOutput(result, "partition", record.PartitionKey)
	putOutput(result, "run_state", record.RunState)
	putOutput(result, "payload", record.Payload)
	putOutput(result, "error", record.Error)
	putOutput(result, "failure_reason", record.FailureReason)
	putOutput(result, "lease_token", record.LeaseToken)
	if record.FencingToken != 0 {
		result["fencing_token"] = record.FencingToken
	}
	if record.MaxActiveMS != 0 {
		result["max_active_ms"] = record.MaxActiveMS
	}
	putOutput(result, "parent_flow_id", record.ParentFlowID)
	putOutput(result, "root_flow_id", record.RootFlowID)
	putOutput(result, "correlation_id", record.CorrelationID)
	if len(record.Attributes) != 0 {
		result["attributes"] = record.Attributes
	}
	if len(record.StateMeta) != 0 {
		result["state_meta"] = record.StateMeta
	}
	if len(record.Values) != 0 {
		result["values"] = record.Values
	}
	if len(record.ValueRefs) != 0 {
		result["value_refs"] = record.ValueRefs
	}
	putOutput(result, "indexed_state_meta", record.IndexedStateMeta)
	if record.ValueSizes != nil {
		result["value_sizes"] = record.ValueSizes
	}
	if record.ValueOmitted != nil {
		result["value_omitted"] = record.ValueOmitted
	}
	if record.ValueMissing != nil {
		result["value_missing"] = record.ValueMissing
	}
	return result
}

func flowRecordsOutput(records []ferricstore.FlowRecord) any {
	result := make([]any, len(records))
	for index := range records {
		result[index] = flowRecordOutput(&records[index])
	}
	return result
}

func flowClaimsOutput(claims []ferricstore.ClaimedItem) any {
	result := make([]any, len(claims))
	for index := range claims {
		claim := claims[index]
		item := map[string]any{
			"id":            claim.ID,
			"lease_token":   claim.LeaseToken,
			"fencing_token": claim.FencingToken,
			"type":          claim.Type,
			"state":         claim.State,
		}
		putOutput(item, "partition_key", claim.PartitionKey)
		putOutput(item, "run_state", claim.RunState)
		putOutput(item, "payload", claim.Payload)
		if len(claim.Attributes) != 0 {
			item["attributes"] = claim.Attributes
		}
		result[index] = item
	}
	return result
}

func putOutput(destination map[string]any, key string, value any) {
	switch typed := value.(type) {
	case nil:
		return
	case string:
		if typed == "" {
			return
		}
	}
	destination[key] = value
}
