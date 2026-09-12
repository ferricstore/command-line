package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/outputcontract"
	"github.com/ferricstore/command-line/internal/profile"
	ferricstore "github.com/ferricstore/ferricstore-go"
	"github.com/spf13/cobra"
)

const (
	flowQueryMaxBytes          = 16 * 1024
	flowQueryMaxParameters     = 64
	flowQueryMaxParameterName  = 128
	flowQueryMaxParameterValue = 65_535
	flowQueryMaxParamsDocument = 5 * 1024 * 1024
)

type flowQuerier interface {
	FlowQuery(context.Context, string, map[string]any) (*ferricstore.FlowQueryResult, error)
}

type flowExplainer interface {
	FlowExplain(context.Context, string, map[string]any) (*ferricstore.FlowExplainResult, error)
	FlowExplainAnalyze(context.Context, string, map[string]any) (*ferricstore.FlowExplainResult, error)
}

type flowQueryIndexReader interface {
	FlowQueryIndexes(context.Context, ...string) (*ferricstore.FlowQueryIndexStatus, error)
}

type flowQueryInputFlags struct {
	queryFile  string
	params     []string
	paramsJSON string
	paramsFile string
}

func newWorkflowQueryCommand(dependencies dependencies) *cobra.Command {
	var flags flowQueryInputFlags
	command := &cobra.Command{
		Use:   "query [fql]",
		Short: "Run a bounded FQL1 workflow query",
		Long: "Run an arbitrary bounded FQL1 query. Supply the query as one shell argument or with --file. " +
			"Use --param for string values or --params-json/--params-file for typed scalar values.",
		Example: "  ferric workflow query 'FROM runs WHERE partition_key = @partition AND type = @type LIMIT 20 RETURN RECORDS' --param partition=tenant-a --param type=order\n" +
			"  ferric workflow query --file failed-orders.fql --params-json '{\"partition\":\"tenant-a\",\"minimum_attempts\":3}'",
		Args: cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			query, params, err := flags.read(command, args)
			if err != nil {
				return err
			}
			if hasFlowExplainPrefix(query) {
				return errors.New("query already contains EXPLAIN; use 'ferric workflow query explain' without the prefix")
			}
			return runNetworkCommand(command, dependencies, "query workflows", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				querier, err := requireClientCapability[flowQuerier](client, "FQL workflow queries")
				if err != nil {
					return nil, err
				}
				result, err := querier.FlowQuery(ctx, query, params)
				if err != nil {
					return nil, formatFlowQueryError(err)
				}
				return outputcontract.FlowQuery(result), nil
			})
		},
	}
	flags.add(command)
	command.AddCommand(
		newWorkflowQueryExplainCommand(dependencies),
		newWorkflowQueryIndexesCommand(dependencies),
	)
	return command
}

func newWorkflowQueryExplainCommand(dependencies dependencies) *cobra.Command {
	var flags flowQueryInputFlags
	var analyze bool
	command := &cobra.Command{
		Use:   "explain [fql]",
		Short: "Inspect an FQL1 workflow query plan",
		Long:  "Plan a bounded FQL1 query without returning records. Use --analyze to execute the admitted plan and include actual usage.",
		Example: "  ferric workflow query explain 'FROM runs WHERE partition_key = @partition AND type = @type LIMIT 20 RETURN RECORDS' --param partition=tenant-a --param type=order\n" +
			"  ferric workflow query explain --file failed-orders.fql --params-file parameters.json --analyze",
		Args: cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			query, params, err := flags.read(command, args)
			if err != nil {
				return err
			}
			if hasFlowExplainPrefix(query) {
				return errors.New("query already contains an EXPLAIN prefix")
			}
			return runNetworkCommand(command, dependencies, "explain workflow query", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				explainer, err := requireClientCapability[flowExplainer](client, "FQL workflow query plans")
				if err != nil {
					return nil, err
				}
				var result *ferricstore.FlowExplainResult
				if analyze {
					result, err = explainer.FlowExplainAnalyze(ctx, query, params)
				} else {
					result, err = explainer.FlowExplain(ctx, query, params)
				}
				if err != nil {
					return nil, formatFlowQueryError(err)
				}
				return outputcontract.FlowExplain(result), nil
			})
		},
	}
	flags.add(command)
	command.Flags().BoolVar(&analyze, "analyze", false, "execute the plan and include actual resource usage")
	return command
}

func newWorkflowQueryIndexesCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "indexes [index-id]",
		Short: "Show FQL query index status",
		Long:  "Show the bounded OSS query-index catalog, lifecycle progress, validation, and statistics. Optionally filter by one logical index ID.",
		Example: "  ferric workflow query indexes\n" +
			"  ferric workflow query indexes flow_type_partition",
		Args: cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if len(args) == 1 && !validFlowQueryIndexID(args[0]) {
				return errors.New("query index ID must be 1..64 ASCII letters, digits, '_', '-', ':', or '.'")
			}
			return runNetworkCommand(command, dependencies, "inspect workflow query indexes", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[flowQueryIndexReader](client, "FQL workflow query indexes")
				if err != nil {
					return nil, err
				}
				result, err := reader.FlowQueryIndexes(ctx, args...)
				if err != nil {
					return nil, formatFlowQueryError(err)
				}
				return outputcontract.FlowQueryIndexes(result), nil
			})
		},
	}
}

func (flags *flowQueryInputFlags) add(command *cobra.Command) {
	command.Flags().StringVar(&flags.queryFile, "file", "", "read the FQL query from a file; use - for stdin")
	command.Flags().StringArrayVar(&flags.params, "param", nil, "string query parameter as name=value; repeatable")
	command.Flags().StringVar(&flags.paramsJSON, "params-json", "", "typed query parameters as a JSON object")
	command.Flags().StringVar(&flags.paramsFile, "params-file", "", "read typed query parameters from a JSON file; use - for stdin")
}

func (flags flowQueryInputFlags) read(command *cobra.Command, args []string) (string, map[string]any, error) {
	if flags.queryFile != "" && len(args) != 0 {
		return "", nil, errors.New("provide a positional query or --file, not both")
	}
	if flags.queryFile == "" && len(args) == 0 {
		return "", nil, errors.New("query is required; provide FQL or use --file <path>")
	}
	if flags.paramsJSON != "" && flags.paramsFile != "" {
		return "", nil, errors.New("use --params-json or --params-file, not both")
	}
	if flags.queryFile == "-" && flags.paramsFile == "-" {
		return "", nil, errors.New("the query and parameters cannot both read from stdin")
	}

	query, err := readFlowQuery(command, flags.queryFile, args)
	if err != nil {
		return "", nil, err
	}
	params, err := flags.readParams(command)
	if err != nil {
		return "", nil, err
	}
	return query, params, nil
}

func readFlowQuery(command *cobra.Command, path string, args []string) (string, error) {
	if path == "" {
		query := args[0]
		if err := validateFlowQueryInput(query); err != nil {
			return "", err
		}
		return query, nil
	}
	data, err := readBoundedQueryInput(command, path, flowQueryMaxBytes, "query")
	if err != nil {
		return "", err
	}
	query := string(data)
	if err := validateFlowQueryInput(query); err != nil {
		return "", err
	}
	return query, nil
}

func validateFlowQueryInput(query string) error {
	if !utf8.ValidString(query) {
		return errors.New("query must be valid UTF-8")
	}
	if strings.TrimSpace(query) == "" {
		return errors.New("query must not be empty")
	}
	if len(query) > flowQueryMaxBytes {
		return fmt.Errorf("query exceeds %d bytes", flowQueryMaxBytes)
	}
	return nil
}

func (flags flowQueryInputFlags) readParams(command *cobra.Command) (map[string]any, error) {
	var params map[string]any
	if flags.paramsJSON != "" {
		if len(flags.paramsJSON) > flowQueryMaxParamsDocument {
			return nil, fmt.Errorf("parameters JSON exceeds %d bytes", flowQueryMaxParamsDocument)
		}
		var err error
		params, err = decodeFlowQueryParams([]byte(flags.paramsJSON))
		if err != nil {
			return nil, err
		}
	} else if flags.paramsFile != "" {
		data, err := readBoundedQueryInput(command, flags.paramsFile, flowQueryMaxParamsDocument, "parameters JSON")
		if err != nil {
			return nil, err
		}
		params, err = decodeFlowQueryParams(data)
		if err != nil {
			return nil, err
		}
	}
	if params == nil {
		params = make(map[string]any)
	}
	for _, assignment := range flags.params {
		name, value, found := strings.Cut(assignment, "=")
		name = strings.TrimSpace(name)
		if !found || name == "" {
			return nil, fmt.Errorf("invalid query parameter %q: use name=value", assignment)
		}
		if err := validateFlowQueryParameter(name, value); err != nil {
			return nil, err
		}
		if _, duplicate := params[name]; duplicate {
			return nil, fmt.Errorf("duplicate query parameter %q", name)
		}
		params[name] = value
	}
	if len(params) > flowQueryMaxParameters {
		return nil, fmt.Errorf("queries accept at most %d named parameters", flowQueryMaxParameters)
	}
	if len(params) == 0 {
		return nil, nil
	}
	return params, nil
}

func decodeFlowQueryParams(data []byte) (map[string]any, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("parameters JSON must be valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("parse parameters JSON: %w", err)
	}
	opening, ok := token.(json.Delim)
	if !ok || opening != '{' {
		return nil, errors.New("parameters JSON must be an object")
	}
	params := make(map[string]any)
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("parse parameters JSON: %w", err)
		}
		name, ok := token.(string)
		if !ok {
			return nil, errors.New("parameters JSON contains a non-string name")
		}
		if _, duplicate := params[name]; duplicate {
			return nil, fmt.Errorf("duplicate query parameter %q", name)
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("parse query parameter %q: %w", name, err)
		}
		value, err = normalizeFlowQueryJSONValue(value)
		if err != nil {
			return nil, fmt.Errorf("query parameter %q: %w", name, err)
		}
		if err := validateFlowQueryParameter(name, value); err != nil {
			return nil, err
		}
		params[name] = value
		if len(params) > flowQueryMaxParameters {
			return nil, fmt.Errorf("queries accept at most %d named parameters", flowQueryMaxParameters)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("parse parameters JSON: %w", err)
	}
	if token, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("parameters JSON contains trailing value %v", token)
		}
		return nil, fmt.Errorf("parse trailing parameters JSON: %w", err)
	}
	return params, nil
}

func normalizeFlowQueryJSONValue(value any) (any, error) {
	switch typed := value.(type) {
	case string, bool:
		return typed, nil
	case json.Number:
		if integer, err := typed.Int64(); err == nil {
			return integer, nil
		}
		floating, err := typed.Float64()
		if err != nil || math.IsInf(floating, 0) || math.IsNaN(floating) {
			return nil, errors.New("number must fit a finite 64-bit value")
		}
		return floating, nil
	default:
		return nil, errors.New("value must be a string, boolean, or number")
	}
}

func validateFlowQueryParameter(name string, value any) error {
	if !validFlowQueryParameterName(name) {
		return fmt.Errorf("query parameter names must be 1..%d ASCII letters, digits, '_', '.', or '-'", flowQueryMaxParameterName)
	}
	if text, ok := value.(string); ok {
		if !utf8.ValidString(text) {
			return fmt.Errorf("query parameter %q must be valid UTF-8", name)
		}
		if len(text) > flowQueryMaxParameterValue {
			return fmt.Errorf("query parameter %q exceeds %d bytes", name, flowQueryMaxParameterValue)
		}
	}
	return nil
}

func validFlowQueryParameterName(name string) bool {
	if len(name) == 0 || len(name) > flowQueryMaxParameterName {
		return false
	}
	for index := 0; index < len(name); index++ {
		value := name[index]
		if (value < 'a' || value > 'z') &&
			(value < 'A' || value > 'Z') &&
			(value < '0' || value > '9') &&
			value != '_' && value != '.' && value != '-' {
			return false
		}
	}
	return true
}

func validFlowQueryIndexID(indexID string) bool {
	if len(indexID) == 0 || len(indexID) > 64 {
		return false
	}
	for index := 0; index < len(indexID); index++ {
		value := indexID[index]
		if (value < 'a' || value > 'z') &&
			(value < 'A' || value > 'Z') &&
			(value < '0' || value > '9') &&
			value != '_' && value != '-' && value != ':' && value != '.' {
			return false
		}
	}
	return true
}

func hasFlowExplainPrefix(query string) bool {
	query = strings.TrimLeft(query, " \t\n\r")
	const keyword = "EXPLAIN"
	if len(query) < len(keyword) || !strings.EqualFold(query[:len(keyword)], keyword) {
		return false
	}
	if len(query) == len(keyword) {
		return true
	}
	next := query[len(keyword)]
	return next == ' ' || next == '\t' || next == '\n' || next == '\r'
}

func readBoundedQueryInput(command *cobra.Command, path string, maximum int64, noun string) ([]byte, error) {
	var reader io.Reader
	var closeInput func() error
	if path == "-" {
		reader = command.InOrStdin()
	} else {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", noun, err)
		}
		reader = file
		closeInput = file.Close
	}
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if closeInput != nil {
		err = errors.Join(err, closeInput())
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", noun, err)
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("%s exceeds %d bytes", noun, maximum)
	}
	return data, nil
}

func formatFlowQueryError(err error) error {
	var queryErr *ferricstore.FlowQueryError
	if !errors.As(err, &queryErr) || queryErr == nil {
		return err
	}
	message := queryErr.Message
	if message == "" {
		message = queryErr.Code
	}
	parts := []string{fmt.Sprintf("%s: %s", queryErr.Code, message)}
	if queryErr.Position != nil {
		parts = append(parts, fmt.Sprintf(
			"line %d, column %d (byte %d)",
			queryErr.Position.Line,
			queryErr.Position.Column,
			queryErr.Position.Byte,
		))
	}
	if queryErr.Detail != "" {
		parts = append(parts, "detail: "+queryErr.Detail)
	}
	if queryErr.Hint != "" {
		parts = append(parts, "hint: "+queryErr.Hint)
	}
	return flowQueryDiagnostic{message: strings.Join(parts, "; "), cause: err}
}

type flowQueryDiagnostic struct {
	message string
	cause   error
}

func (e flowQueryDiagnostic) Error() string { return e.message }

func (e flowQueryDiagnostic) Unwrap() error { return e.cause }
