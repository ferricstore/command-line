package cli

import (
	"bytes"
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

const maxValueInputBytes = 16 * 1024 * 1024

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
		data, err = readBoundedValueInput(command, input.file)
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

func readBoundedValueInput(command *cobra.Command, path string) ([]byte, error) {
	var (
		reader io.Reader
		file   *os.File
	)
	if path == "-" {
		reader = command.InOrStdin()
	} else {
		var err error
		file, err = os.Open(path)
		if err != nil {
			return nil, err
		}
		reader = file
	}
	data, readErr := io.ReadAll(io.LimitReader(reader, maxValueInputBytes+1))
	if file != nil {
		readErr = errors.Join(readErr, file.Close())
	}
	if readErr != nil {
		return nil, readErr
	}
	if len(data) > maxValueInputBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", maxValueInputBytes)
	}
	return data, nil
}

func readBoundedJSONFile[T any](command *cobra.Command, path, noun string) (T, error) {
	var value T
	if strings.TrimSpace(path) == "" {
		return value, fmt.Errorf("file is required; use --file <path> or --file - for %s", noun)
	}
	data, err := readBoundedValueInput(command, path)
	if err != nil {
		return value, fmt.Errorf("read %s: %w", noun, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("parse %s JSON: %w", noun, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return value, fmt.Errorf("parse %s JSON: multiple JSON values are not allowed", noun)
		}
		return value, fmt.Errorf("parse %s JSON: %w", noun, err)
	}
	return value, nil
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

func parseStateMetaAssignments(values []string) (map[string]map[string]any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string]map[string]any)
	for _, item := range values {
		path, value, found := strings.Cut(item, "=")
		state, name, separated := strings.Cut(strings.TrimSpace(path), ".")
		state = strings.TrimSpace(state)
		name = strings.TrimSpace(name)
		if !found || !separated || state == "" || name == "" {
			return nil, fmt.Errorf("invalid state metadata %q: use state.name=value", item)
		}
		metadata := result[state]
		if metadata == nil {
			metadata = make(map[string]any)
			result[state] = metadata
		}
		if _, duplicate := metadata[name]; duplicate {
			return nil, fmt.Errorf("duplicate state metadata %q", state+"."+name)
		}
		metadata[name] = value
	}
	return result, nil
}
