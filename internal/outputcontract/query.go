package outputcontract

import ferricstore "github.com/ferricstore/ferricstore-go"

// FlowQuery converts an ordinary FQL result using the CLI's stable schema.
func FlowQuery(value *ferricstore.FlowQueryResult) any {
	if value == nil {
		return nil
	}
	output := map[string]any{
		"version": value.Version,
		"quality": flowQueryQuality(value.Quality),
		"usage":   flowQueryUsage(value.Usage),
	}
	if value.Records != nil {
		output["records"] = value.Records
	}
	if value.Page != nil {
		output["page"] = flowQueryPage(*value.Page)
	}
	if value.Count != nil {
		output["result"] = map[string]any{"kind": "count", "value": *value.Count}
	}
	return output
}

// FlowExplain converts an FQL plan or analyzed plan without SDK Raw fields.
func FlowExplain(value *ferricstore.FlowExplainResult) any {
	if value == nil {
		return nil
	}
	output := map[string]any{
		"version":           value.Version,
		"query_fingerprint": value.QueryFingerprint,
		"status":            value.Status,
		"plan":              value.Plan,
		"estimate":          value.Estimate,
		"stats":             value.Stats,
		"quality":           nil,
		"bounds":            value.Bounds,
		"pressure":          value.Pressure,
		"decision":          value.Decision,
		"alternatives":      value.Alternatives,
		"capabilities":      nil,
		"actual":            nil,
		"diagnostic":        nil,
	}
	if value.Quality != nil {
		output["quality"] = flowQueryQuality(*value.Quality)
	}
	if value.Capabilities != nil {
		output["capabilities"] = map[string]any{
			"requested": value.Capabilities.Requested,
			"available": value.Capabilities.Available,
			"missing":   value.Capabilities.Missing,
		}
	}
	if value.Actual != nil {
		output["actual"] = flowQueryUsage(*value.Actual)
	}
	if value.Diagnostic != nil {
		output["diagnostic"] = flowQueryDiagnostic(value.Diagnostic)
	}
	return output
}

func flowQueryPage(value ferricstore.FlowQueryPage) map[string]any {
	return map[string]any{"has_more": value.HasMore, "cursor": value.Cursor}
}

func flowQueryQuality(value ferricstore.FlowQueryQuality) map[string]any {
	return map[string]any{
		"exactness":  value.Exactness,
		"freshness":  value.Freshness,
		"coverage":   value.Coverage,
		"pagination": value.Pagination,
	}
}

func flowQueryUsage(value ferricstore.FlowQueryUsage) map[string]any {
	return map[string]any{
		"range_seeks":             value.RangeSeeks,
		"range_pages":             value.RangePages,
		"scanned_entries":         value.ScannedEntries,
		"scanned_bytes":           value.ScannedBytes,
		"hydrated_records":        value.HydratedRecords,
		"residual_checks":         value.ResidualChecks,
		"duplicate_entries":       value.DuplicateEntries,
		"result_records":          value.ResultRecords,
		"response_bytes":          value.ResponseBytes,
		"memory_high_water_bytes": value.MemoryHighWaterBytes,
		"wall_time_us":            value.WallTimeUS,
	}
}

func flowQueryDiagnostic(value *ferricstore.FlowQueryError) map[string]any {
	output := map[string]any{
		"code":           value.Code,
		"message":        value.Message,
		"detail":         value.Detail,
		"hint":           value.Hint,
		"retryable":      value.Retryable,
		"safe_to_retry":  value.SafeToRetry,
		"retry_after_ms": value.RetryAfterMS,
		"position":       nil,
		"context":        value.Context,
	}
	if value.Position != nil {
		output["position"] = map[string]any{
			"byte":   value.Position.Byte,
			"line":   value.Position.Line,
			"column": value.Position.Column,
		}
	}
	return output
}

// FlowQueryIndexes converts the complete OSS FQL index contract while
// intentionally excluding SDK transport snapshots.
func FlowQueryIndexes(value *ferricstore.FlowQueryIndexStatus) any {
	if value == nil {
		return nil
	}
	indexes := make([]any, len(value.Indexes))
	for index := range value.Indexes {
		indexes[index] = flowQueryIndex(value.Indexes[index])
	}
	return map[string]any{
		"contract_version":      value.ContractVersion,
		"observed_at_ms":        value.ObservedAtMS,
		"statistics_max_age_ms": value.StatisticsMaxAgeMS,
		"registry": map[string]any{
			"epoch":           value.Registry.Epoch,
			"catalog_version": value.Registry.CatalogVersion,
		},
		"services": map[string]any{
			"registry":          value.Services.Registry,
			"lifecycle_worker":  value.Services.LifecycleWorker,
			"statistics_store":  value.Services.StatisticsStore,
			"statistics_worker": value.Services.StatisticsWorker,
		},
		"indexes": indexes,
	}
}

func flowQueryIndex(value ferricstore.FlowQueryIndex) map[string]any {
	fields := make([]any, len(value.Fields))
	for index := range value.Fields {
		fields[index] = map[string]any{
			"name":      value.Fields[index].Name,
			"direction": value.Fields[index].Direction,
			"encoding":  value.Fields[index].Encoding,
		}
	}
	return map[string]any{
		"id":              value.ID,
		"version":         value.Version,
		"build_id":        value.BuildID,
		"source":          value.Source,
		"state":           value.State,
		"queryable":       value.Queryable,
		"fields":          fields,
		"workloads":       value.Workloads,
		"count_prefixes":  value.CountPrefixes,
		"covering_fields": value.CoveringFields,
		"format": map[string]any{
			"query_row": value.Format.QueryRow,
			"key":       value.Format.Key,
			"entry":     value.Format.Entry,
			"reverse":   value.Format.Reverse,
			"counter":   value.Format.Counter,
		},
		"coverage": map[string]any{
			"complete_shards": value.Coverage.CompleteShards,
			"total_shards":    value.Coverage.TotalShards,
			"validation":      value.Coverage.Validation,
		},
		"build":      flowQueryIndexBuild(value.Build),
		"validation": flowQueryIndexValidation(value.Validation),
		"retirement": flowQueryIndexRetirement(value.Retirement),
		"statistics": flowQueryIndexStatistics(value.Statistics),
	}
}

func flowQueryIndexBuild(value ferricstore.FlowQueryIndexBuild) map[string]any {
	return map[string]any{
		"scope":            value.Scope,
		"phase_counts":     value.PhaseCounts,
		"current_phases":   value.CurrentPhases,
		"completed_shards": value.CompletedShards,
		"total_shards":     value.TotalShards,
		"scanned_records":  value.ScannedRecords,
		"written_entries":  value.WrittenEntries,
		"written_bytes":    value.WrittenBytes,
	}
}

func flowQueryIndexValidation(value ferricstore.FlowQueryIndexValidation) map[string]any {
	return map[string]any{
		"scope":            value.Scope,
		"status":           value.Status,
		"phase_counts":     value.PhaseCounts,
		"current_phases":   value.CurrentPhases,
		"completed_shards": value.CompletedShards,
		"total_shards":     value.TotalShards,
		"checked_records":  value.CheckedRecords,
		"checked_entries":  value.CheckedEntries,
		"mismatches":       value.Mismatches,
		"failure_reason":   value.FailureReason,
		"validated_at_ms":  value.ValidatedAtMS,
	}
}

func flowQueryIndexRetirement(value ferricstore.FlowQueryIndexRetirement) map[string]any {
	return map[string]any{
		"status":                 value.Status,
		"phase_counts":           value.PhaseCounts,
		"current_phases":         value.CurrentPhases,
		"completed_shards":       value.CompletedShards,
		"total_shards":           value.TotalShards,
		"deleted_entries":        value.DeletedEntries,
		"deleted_bytes":          value.DeletedBytes,
		"rewritten_reverse_rows": value.RewrittenReverseRows,
	}
}

func flowQueryIndexStatistics(value ferricstore.FlowQueryIndexStatistics) map[string]any {
	return map[string]any{
		"status":                 value.Status,
		"samples":                value.Samples,
		"fresh_samples":          value.FreshSamples,
		"stale_samples":          value.StaleSamples,
		"future_samples":         value.FutureSamples,
		"oldest_collected_at_ms": value.OldestCollectedAtMS,
		"newest_collected_at_ms": value.NewestCollectedAtMS,
		"oldest_age_ms":          value.OldestAgeMS,
		"newest_age_ms":          value.NewestAgeMS,
	}
}
