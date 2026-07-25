package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode"
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
		return string(value.Bytes())
	}
	switch value.Kind() {
	case reflect.Struct:
		result := make(map[string]any)
		typeOfValue := value.Type()
		for index := 0; index < value.NumField(); index++ {
			field := typeOfValue.Field(index)
			if !field.IsExported() {
				continue
			}
			name := outputFieldName(field)
			if name == "-" {
				continue
			}
			result[name] = normalizeOutputValue(value.Field(index))
		}
		return result
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

func outputFieldName(field reflect.StructField) string {
	if tag := strings.Split(field.Tag.Get("json"), ",")[0]; tag != "" {
		return tag
	}
	return snakeCase(field.Name)
}

func snakeCase(value string) string {
	var result []rune
	runes := []rune(value)
	for index, current := range runes {
		if unicode.IsUpper(current) {
			if index > 0 && (unicode.IsLower(runes[index-1]) ||
				(index+1 < len(runes) && unicode.IsLower(runes[index+1]))) {
				result = append(result, '_')
			}
			current = unicode.ToLower(current)
		}
		result = append(result, current)
	}
	return string(result)
}

// compactOutput omits zero-valued SDK response fields and transport-level Raw
// maps. It keeps operator output focused while preserving all populated fields.
func compactOutput(value any) any {
	return compactOutputValue(reflect.ValueOf(value))
}

func compactOutputValue(value reflect.Value) any {
	if !value.IsValid() {
		return nil
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		return compactOutputValue(value.Elem())
	}
	if value.Type() == reflect.TypeOf([]byte(nil)) {
		return string(value.Bytes())
	}
	switch value.Kind() {
	case reflect.Struct:
		result := make(map[string]any)
		typeOfValue := value.Type()
		for index := 0; index < value.NumField(); index++ {
			field := typeOfValue.Field(index)
			fieldValue := value.Field(index)
			name := outputFieldName(field)
			if !field.IsExported() || name == "-" || name == "raw" || fieldValue.IsZero() {
				continue
			}
			result[name] = compactOutputValue(fieldValue)
		}
		return result
	case reflect.Slice, reflect.Array:
		result := make([]any, value.Len())
		for index := 0; index < value.Len(); index++ {
			result[index] = compactOutputValue(value.Index(index))
		}
		return result
	case reflect.Map:
		return normalizeOutputValue(value)
	default:
		return normalizeOutputValue(value)
	}
}
