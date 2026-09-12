package cli

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

type outputFormat string

const (
	outputAuto outputFormat = "auto"
	outputJSON outputFormat = "json"
	outputRaw  outputFormat = "raw"
)

func (f *outputFormat) Set(value string) error {
	normalized := outputFormat(strings.ToLower(strings.TrimSpace(value)))
	switch normalized {
	case outputAuto, outputJSON, outputRaw:
		*f = normalized
		return nil
	default:
		return fmt.Errorf("invalid output format %q: use auto, json, or raw", value)
	}
}

func (f outputFormat) String() string {
	if f == "" {
		return string(outputAuto)
	}
	return string(f)
}

func (f outputFormat) Type() string { return "format" }

func writeResult(writer io.Writer, format outputFormat, value any) error {
	if writer == nil {
		return errors.New("output writer is not configured")
	}
	if err := validateExplicitOutputContract(reflect.ValueOf(value)); err != nil {
		return err
	}
	value = normalizeOutputValue(reflect.ValueOf(value))
	switch format {
	case "", outputAuto:
		if isScalar(value) {
			return writeScalar(writer, value, true)
		}
		return writeJSON(writer, value)
	case outputJSON:
		return writeJSON(writer, value)
	case outputRaw:
		return writeRaw(writer, value)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func validateExplicitOutputContract(value reflect.Value) error {
	if !value.IsValid() {
		return nil
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		return validateExplicitOutputContract(value.Elem())
	}
	if value.Type() == reflect.TypeOf([]byte(nil)) {
		return nil
	}
	switch value.Kind() {
	case reflect.Struct:
		return fmt.Errorf("output type %s requires an explicit output contract", value.Type())
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			if err := validateExplicitOutputContract(iterator.Value()); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if err := validateExplicitOutputContract(value.Index(index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeRaw(writer io.Writer, value any) error {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if isScalar(item) {
				if err := writeScalar(writer, item, false); err != nil {
					return err
				}
				continue
			}
			encoded, err := json.Marshal(item)
			if err != nil {
				return err
			}
			if _, err := writer.Write(append(encoded, '\n')); err != nil {
				return err
			}
		}
		return nil
	default:
		if isScalar(value) {
			return writeScalar(writer, value, false)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = writer.Write(append(encoded, '\n'))
		return err
	}
}

func writeScalar(writer io.Writer, value any, visibleNil bool) error {
	var text string
	switch typed := value.(type) {
	case nil:
		if visibleNil {
			text = "(nil)"
		}
	case string:
		text = typed
	case bool:
		text = strconv.FormatBool(typed)
	default:
		text = fmt.Sprint(typed)
	}
	_, err := fmt.Fprintln(writer, text)
	return err
}

func isScalar(value any) bool {
	switch value.(type) {
	case nil, string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	default:
		return false
	}
}

func normalizeOutputValue(value reflect.Value) any {
	if !value.IsValid() {
		return nil
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		return normalizeOutputValue(value.Elem())
	}
	if value.Type() == reflect.TypeOf([]byte(nil)) {
		contents := value.Bytes()
		if utf8.Valid(contents) {
			return string(contents)
		}
		return map[string]any{
			"encoding": "base64",
			"data":     base64.StdEncoding.EncodeToString(contents),
		}
	}
	switch value.Kind() {
	case reflect.Map:
		result := make(map[string]any, value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			result[fmt.Sprint(iterator.Key().Interface())] = normalizeOutputValue(iterator.Value())
		}
		return result
	case reflect.Slice, reflect.Array:
		result := make([]any, value.Len())
		for index := 0; index < value.Len(); index++ {
			result[index] = normalizeOutputValue(value.Index(index))
		}
		return result
	case reflect.String:
		return value.String()
	case reflect.Bool:
		return value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value.Uint()
	case reflect.Float32, reflect.Float64:
		return value.Float()
	default:
		return value.Interface()
	}
}
