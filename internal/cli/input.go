package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type valueInput struct {
	file string
	json bool
}

func (input *valueInput) addFlags(command *cobra.Command, noun string) {
	command.Flags().StringVar(&input.file, "file", "", "read "+noun+" from a file; use - for stdin")
	command.Flags().BoolVar(&input.json, "json", false, "parse "+noun+" as JSON before sending it")
}

func (input valueInput) read(command *cobra.Command, positional *string) (any, bool, error) {
	if input.file != "" && positional != nil {
		return nil, false, errors.New("provide a positional value or --file, not both")
	}
	var (
		data    []byte
		present bool
		err     error
	)
	if input.file != "" {
		present = true
		if input.file == "-" {
			data, err = io.ReadAll(command.InOrStdin())
		} else {
			data, err = os.ReadFile(input.file)
		}
		if err != nil {
			return nil, false, fmt.Errorf("read input: %w", err)
		}
	} else if positional != nil {
		present = true
		data = []byte(*positional)
	}
	if !present {
		if input.json {
			return nil, false, errors.New("--json requires a positional value or --file")
		}
		return nil, false, nil
	}
	if !input.json {
		return string(data), true, nil
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, false, fmt.Errorf("parse JSON input: %w", err)
	}
	return value, true, nil
}

func parseAssignments(values []string, noun string) (map[string]any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string]any, len(values))
	for _, item := range values {
		key, value, found := strings.Cut(item, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, fmt.Errorf("invalid %s %q: use name=value", noun, item)
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate %s %q", noun, key)
		}
		result[key] = value
	}
	return result, nil
}
