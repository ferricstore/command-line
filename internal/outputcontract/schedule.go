package outputcontract

import ferricstore "github.com/ferricstore/ferricstore-go"

// ScheduleFireDue converts a scheduler batch result without its Raw snapshot.
func ScheduleFireDue(value ferricstore.ScheduleFireDueResult) any {
	output := map[string]any{}
	putInt64(output, "claimed", value.Claimed)
	putInt64(output, "fired", value.Fired)
	putInt64(output, "skipped", value.Skipped)
	putInt64(output, "coalesced", value.Coalesced)
	if value.Errors != nil {
		errors := make([]any, len(value.Errors))
		for index := range value.Errors {
			item := map[string]any{}
			putString(item, "id", value.Errors[index].ID)
			putString(item, "reason", value.Errors[index].Reason)
			errors[index] = item
		}
		output["errors"] = errors
	}
	putString(output, "claim_error", value.ClaimError)
	putString(output, "last_target_id", value.LastTargetID)
	putString(output, "last_skip_reason", value.LastSkipReason)
	return output
}
