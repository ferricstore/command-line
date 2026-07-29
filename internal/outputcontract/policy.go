package outputcontract

import ferricstore "github.com/ferricstore/ferricstore-go"

// Policy converts an effective FerricFlow policy without exposing Raw SDK
// snapshots. Zero and nil values remain present because they are meaningful in
// this configuration schema.
func Policy(value ferricstore.PolicySnapshot) any {
	states := make(map[string]any, len(value.States))
	for name, state := range value.States {
		states[name] = map[string]any{
			"mode":          state.Mode,
			"max_active_ms": state.MaxActiveMS,
			"retry":         state.Retry,
			"retention":     state.Retention,
			"governance":    state.Governance,
		}
	}
	if value.States == nil {
		states = nil
	}
	return map[string]any{
		"type":               value.Type,
		"state":              value.State,
		"generation":         value.Generation,
		"version":            value.Version,
		"mode":               value.Mode,
		"max_active_ms":      value.MaxActiveMS,
		"retry":              value.Retry,
		"retention":          value.Retention,
		"indexed_attributes": value.IndexedAttributes,
		"indexed_state_meta": value.IndexedStateMeta,
		"governance":         value.Governance,
		"states":             states,
	}
}
