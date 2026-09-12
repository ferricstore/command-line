package cli

import (
	"errors"
	"fmt"
	"strings"
)

const maxFlowBatchItems = 100_000

func validateCreateManyInput(input createManyInput) error {
	if strings.TrimSpace(input.Type) == "" {
		return errors.New("type is required")
	}
	if err := validateBatchSize(len(input.Items)); err != nil {
		return err
	}
	allowDuplicates := input.Independent != nil && *input.Independent
	seen := make(map[string]struct{}, len(input.Items))
	for index, item := range input.Items {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("items[%d].id is required", index)
		}
		if !allowDuplicates {
			if _, duplicate := seen[item.ID]; duplicate {
				return fmt.Errorf("duplicate item id %q", item.ID)
			}
			seen[item.ID] = struct{}{}
		}
	}
	if input.NowMS < 0 {
		return errors.New("now_ms must not be negative")
	}
	if input.RunAtMS < 0 {
		return errors.New("run_at_ms must not be negative")
	}
	if input.RetentionTTLMS != nil && *input.RetentionTTLMS <= 0 {
		return errors.New("retention_ttl_ms must be greater than zero")
	}
	return nil
}

func validateClaimedItemsInput(items []claimedItemInput, independent *bool) error {
	if err := validateBatchSize(len(items)); err != nil {
		return err
	}
	allowDuplicates := independent != nil && *independent
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("items[%d].id is required", index)
		}
		if strings.TrimSpace(item.LeaseToken) == "" {
			return fmt.Errorf("items[%d].lease_token is required", index)
		}
		if item.FencingToken < 0 {
			return fmt.Errorf("items[%d].fencing_token must not be negative", index)
		}
		if !allowDuplicates {
			if _, duplicate := seen[item.ID]; duplicate {
				return fmt.Errorf("duplicate item id %q", item.ID)
			}
			seen[item.ID] = struct{}{}
		}
	}
	return nil
}

func validateFencedItemsInput(items []fencedItemInput, independent *bool, requireLease bool) error {
	if err := validateBatchSize(len(items)); err != nil {
		return err
	}
	allowDuplicates := independent != nil && *independent
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("items[%d].id is required", index)
		}
		if requireLease && strings.TrimSpace(item.LeaseToken) == "" {
			return fmt.Errorf("items[%d].lease_token is required", index)
		}
		if item.FencingToken < 0 {
			return fmt.Errorf("items[%d].fencing_token must not be negative", index)
		}
		if !allowDuplicates {
			if _, duplicate := seen[item.ID]; duplicate {
				return fmt.Errorf("duplicate item id %q", item.ID)
			}
			seen[item.ID] = struct{}{}
		}
	}
	return nil
}

func validateTransitionManyInput(input transitionManyInput) error {
	if strings.TrimSpace(input.FromState) == "" {
		return errors.New("from_state is required")
	}
	if strings.TrimSpace(input.ToState) == "" {
		return errors.New("to_state is required")
	}
	if input.ToState == "running" {
		return errors.New("to_state running is only entered by claim")
	}
	if input.NowMS < 0 {
		return errors.New("now_ms must not be negative")
	}
	if input.RunAtMS < 0 {
		return errors.New("run_at_ms must not be negative")
	}
	return validateFencedItemsInput(input.Items, input.Independent, true)
}

func validateRunStepsManyInput(input runStepsManyInput) error {
	if err := validateBatchSize(len(input.Items)); err != nil {
		return err
	}
	if strings.TrimSpace(input.Type) == "" {
		return errors.New("type is required")
	}
	if strings.TrimSpace(input.Worker) == "" {
		return errors.New("worker is required")
	}
	if (len(input.States) == 0) == (input.Steps == 0) {
		return errors.New("exactly one of states or steps is required")
	}
	if input.Steps < 0 {
		return errors.New("steps must not be negative")
	}
	if input.LeaseMS < 0 {
		return errors.New("lease_ms must not be negative")
	}
	if input.NowMS < 0 {
		return errors.New("now_ms must not be negative")
	}
	if input.RetentionTTLMS != nil && *input.RetentionTTLMS <= 0 {
		return errors.New("retention_ttl_ms must be greater than zero")
	}
	for index, state := range input.States {
		if strings.TrimSpace(state) == "" {
			return fmt.Errorf("states[%d] is required", index)
		}
	}
	seen := make(map[string]struct{}, len(input.Items))
	for index, item := range input.Items {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("items[%d].id is required", index)
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return fmt.Errorf("duplicate item id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	return nil
}

func validateBatchSize(count int) error {
	if count == 0 {
		return errors.New("batch requires at least one item")
	}
	if count > maxFlowBatchItems {
		return fmt.Errorf("batch exceeds %d items", maxFlowBatchItems)
	}
	return nil
}
