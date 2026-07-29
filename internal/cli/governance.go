package cli

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ferricstore/command-line/internal/connection"
	"github.com/ferricstore/command-line/internal/outputcontract"
	"github.com/ferricstore/command-line/internal/profile"
	ferricstore "github.com/ferricstore/ferricstore-go"
	"github.com/spf13/cobra"
)

func newGovernanceCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "governance",
		Short: "Inspect and operate FerricFlow governance controls",
		Long:  "Manage approvals, circuit breakers, governed effects, budgets, and distributed limits.",
		Example: "  ferric workflow governance overview --scope tenant-a\n" +
			"  ferric workflow governance approval list --status pending\n" +
			"  ferric workflow governance circuit get payments",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newGovernanceOverviewCommand(dependencies),
		newApprovalCommand(dependencies),
		newCircuitCommand(dependencies),
		newBudgetCommand(dependencies),
		newLimitCommand(dependencies),
		newEffectCommand(dependencies),
	)
	return command
}

type governanceOverviewReader interface {
	GovernanceOverview(context.Context, ferricstore.ApprovalListOptions) (ferricstore.GovernanceOverview, error)
}

func newGovernanceOverviewCommand(dependencies dependencies) *cobra.Command {
	var status string
	var scope string
	var partition string
	var flowID string
	var limit int
	command := &cobra.Command{
		Use:   "overview",
		Short: "Show a bounded governance summary",
		Example: "  ferric workflow governance overview --scope tenant-a\n" +
			"  ferric workflow governance overview --partition tenant-a --status pending --limit 20",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			options, err := governanceListOptions(status, scope, partition, flowID, limit)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "governance overview", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				reader, err := requireClientCapability[governanceOverviewReader](client, "governance overview")
				if err != nil {
					return nil, err
				}
				result, err := reader.GovernanceOverview(ctx, options)
				return outputcontract.GovernanceOverview(result), err
			})
		},
	}
	addGovernanceListFlags(command, &status, &scope, &partition, &flowID, &limit)
	return command
}

func addGovernanceListFlags(command *cobra.Command, status, scope, partition, flowID *string, limit *int) {
	command.Flags().StringVar(status, "status", "", "filter by status")
	command.Flags().StringVar(scope, "scope", "", "governance scope")
	command.Flags().StringVar(partition, "partition", "", "partition key")
	command.Flags().StringVar(flowID, "flow-id", "", "filter by workflow ID")
	command.Flags().IntVar(limit, "limit", 100, "maximum items per collection")
}

func governanceListOptions(status, scope, partition, flowID string, limit int) (ferricstore.ApprovalListOptions, error) {
	if limit <= 0 {
		return ferricstore.ApprovalListOptions{}, errors.New("limit must be greater than zero")
	}
	if scope != "" && partition != "" {
		return ferricstore.ApprovalListOptions{}, errors.New("use --scope or --partition, not both")
	}
	return ferricstore.ApprovalListOptions{
		Status: status, Scope: scope, PartitionKey: partition, FlowID: flowID, Limit: ferricstore.Int(limit),
	}, nil
}

func newApprovalCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "approval",
		Short: "Request, inspect, and decide approvals",
		Example: "  ferric workflow governance approval list --status pending\n" +
			"  ferric workflow governance approval get approval-42",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newApprovalRequestCommand(dependencies),
		newApprovalGetCommand(dependencies),
		newApprovalListCommand(dependencies),
		newApprovalDecisionCommand(dependencies, true),
		newApprovalDecisionCommand(dependencies, false),
	)
	return command
}

type approvalRequester interface {
	ApprovalRequest(context.Context, string, ferricstore.ApprovalRequestOptions) (ferricstore.ApprovalResult, error)
}

func newApprovalRequestCommand(dependencies dependencies) *cobra.Command {
	var flowID string
	var scope string
	var reason string
	var requestedBy string
	var assignees []string
	var policyHash string
	var policyVersion string
	var timeout time.Duration
	var expiresAt string
	command := &cobra.Command{
		Use:     "request <id>",
		Short:   "Request a governed approval",
		Example: "  ferric workflow governance approval request approval-42 --flow-id order-42 --scope payments --reason 'large payment' --requested-by worker-1 --assignee finance",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(flowID) == "" {
				return errors.New("flow-id is required; use --flow-id <id>")
			}
			if strings.TrimSpace(scope) == "" {
				return errors.New("scope is required; use --scope <scope>")
			}
			options := ferricstore.ApprovalRequestOptions{
				FlowID: flowID, Scope: scope, Reason: reason, RequestedBy: requestedBy,
				Assignees: assignees, PolicyHash: policyHash,
			}
			if command.Flags().Changed("policy-version") {
				options.PolicyVersion = policyVersion
			}
			if command.Flags().Changed("timeout-after") {
				value, err := positiveMilliseconds("timeout", timeout)
				if err != nil {
					return err
				}
				options.TimeoutMS = &value
			}
			if expiresAt != "" {
				value, err := parseAbsoluteMilliseconds("expires-at", expiresAt)
				if err != nil {
					return err
				}
				options.ExpiresAtMS = &value
			}
			return runNetworkCommand(command, dependencies, "request approval", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				requester, err := requireClientCapability[approvalRequester](client, "approval requests")
				if err != nil {
					return nil, err
				}
				result, err := requester.ApprovalRequest(ctx, args[0], options)
				return outputcontract.Approval(&result), err
			})
		},
	}
	command.Flags().StringVar(&flowID, "flow-id", "", "workflow ID associated with the approval")
	command.Flags().StringVar(&scope, "scope", "", "governance scope")
	command.Flags().StringVar(&reason, "reason", "", "reason approval is needed")
	command.Flags().StringVar(&requestedBy, "requested-by", "", "requesting principal")
	command.Flags().StringSliceVar(&assignees, "assignee", nil, "eligible approver; repeatable")
	command.Flags().StringVar(&policyHash, "policy-hash", "", "governance policy hash")
	command.Flags().StringVar(&policyVersion, "policy-version", "", "governance policy version")
	command.Flags().DurationVar(&timeout, "timeout-after", 0, "approval timeout duration")
	command.Flags().StringVar(&expiresAt, "expires-at", "", "absolute expiry as RFC3339 or Unix milliseconds")
	return command
}

type approvalGetter interface {
	ApprovalGet(context.Context, string) (*ferricstore.ApprovalResult, error)
}

func newApprovalGetCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "get <id>",
		Aliases: []string{"describe", "inspect"},
		Short:   "Show one approval",
		Example: "  ferric workflow governance approval get approval-42",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "get approval", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				getter, err := requireClientCapability[approvalGetter](client, "approval reads")
				if err != nil {
					return nil, err
				}
				result, err := getter.ApprovalGet(ctx, args[0])
				return outputcontract.Approval(result), err
			})
		},
	}
}

type approvalLister interface {
	ApprovalList(context.Context, ferricstore.ApprovalListOptions) ([]ferricstore.ApprovalResult, error)
}

func newApprovalListCommand(dependencies dependencies) *cobra.Command {
	var status string
	var scope string
	var partition string
	var flowID string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List approvals",
		Example: "  ferric workflow governance approval list --status pending --scope payments\n" +
			"  ferric workflow governance approval list --flow-id order-42 --limit 20",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			options, err := governanceListOptions(status, scope, partition, flowID, limit)
			if err != nil {
				return err
			}
			return runNetworkCommand(command, dependencies, "list approvals", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				lister, err := requireClientCapability[approvalLister](client, "approval list")
				if err != nil {
					return nil, err
				}
				result, err := lister.ApprovalList(ctx, options)
				return outputcontract.ApprovalList(result), err
			})
		},
	}
	addGovernanceListFlags(command, &status, &scope, &partition, &flowID, &limit)
	_ = command.RegisterFlagCompletionFunc("status", completeApprovalStatus)
	return command
}

func completeApprovalStatus(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"pending", "approved", "rejected", "expired"}, cobra.ShellCompDirectiveNoFileComp
}

type approvalApprover interface {
	ApprovalApprove(context.Context, string, string, string, *int64) (ferricstore.ApprovalResult, error)
}

type approvalRejecter interface {
	ApprovalReject(context.Context, string, string, string, *int64) (ferricstore.ApprovalResult, error)
}

func newApprovalDecisionCommand(dependencies dependencies, approve bool) *cobra.Command {
	name := "reject"
	short := "Reject an approval"
	if approve {
		name = "approve"
		short = "Approve a pending approval"
	}
	var approver string
	var reason string
	command := &cobra.Command{
		Use:     name + " <id>",
		Short:   short,
		Example: "  ferric workflow governance approval " + name + " approval-42 --approver ada --reason reviewed",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(approver) == "" {
				return errors.New("approver is required; use --approver <name>")
			}
			return runNetworkCommand(command, dependencies, name+" approval", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				if approve {
					operator, err := requireClientCapability[approvalApprover](client, "approval decisions")
					if err != nil {
						return nil, err
					}
					result, err := operator.ApprovalApprove(ctx, args[0], approver, reason, nil)
					return outputcontract.Approval(&result), err
				}
				operator, err := requireClientCapability[approvalRejecter](client, "approval decisions")
				if err != nil {
					return nil, err
				}
				result, err := operator.ApprovalReject(ctx, args[0], approver, reason, nil)
				return outputcontract.Approval(&result), err
			})
		},
	}
	command.Flags().StringVar(&approver, "approver", "", "approving principal (required)")
	command.Flags().StringVar(&reason, "reason", "", "decision reason")
	return command
}

func newCircuitCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "circuit",
		Short: "Inspect and operate circuit breakers",
		Example: "  ferric workflow governance circuit get payments\n" +
			"  ferric workflow governance circuit open payments --open-for 1m --yes",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newCircuitGetCommand(dependencies),
		newCircuitOpenCommand(dependencies),
		newCircuitCloseCommand(dependencies),
	)
	return command
}

type circuitGetter interface {
	CircuitGet(context.Context, string) (*ferricstore.CircuitBreakerStatus, error)
}

func newCircuitGetCommand(dependencies dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "get <scope>",
		Aliases: []string{"describe", "inspect"},
		Short:   "Show one circuit breaker",
		Example: "  ferric workflow governance circuit get payments",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runNetworkCommand(command, dependencies, "get circuit", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				getter, err := requireClientCapability[circuitGetter](client, "circuit reads")
				if err != nil {
					return nil, err
				}
				result, err := getter.CircuitGet(ctx, args[0])
				return outputcontract.Circuit(result), err
			})
		},
	}
}

type circuitOpener interface {
	CircuitOpenWithOptions(context.Context, string, ferricstore.CircuitOpenOptions) (ferricstore.CircuitBreakerStatus, error)
}

func newCircuitOpenCommand(dependencies dependencies) *cobra.Command {
	var yes bool
	var openFor time.Duration
	var failureThreshold int64
	var window time.Duration
	var minCalls int64
	var failureRate int64
	var latency time.Duration
	var errorClasses []string
	var maxProbes int64
	var successThreshold int64
	command := &cobra.Command{
		Use:   "open <scope>",
		Short: "Open or reconfigure a circuit breaker",
		Long:  "Open or reconfigure a governed circuit. This affects traffic and requires --yes.",
		Example: "  ferric workflow governance circuit open payments --open-for 1m --failure-threshold 5 --yes\n" +
			"  ferric workflow governance circuit open payments --window 5m --min-calls 20 --failure-rate 50 --yes",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !yes {
				return errors.New("opening a circuit requires --yes")
			}
			options := ferricstore.CircuitOpenOptions{ErrorClasses: errorClasses}
			var err error
			if command.Flags().Changed("open-for") {
				value, durationErr := positiveMilliseconds("open-for", openFor)
				err = durationErr
				options.OpenMS = &value
			}
			if err != nil {
				return err
			}
			if command.Flags().Changed("window") {
				value, durationErr := positiveMilliseconds("window", window)
				if durationErr != nil {
					return durationErr
				}
				options.WindowMS = &value
			}
			if command.Flags().Changed("latency-threshold") {
				value, durationErr := positiveMilliseconds("latency-threshold", latency)
				if durationErr != nil {
					return durationErr
				}
				options.LatencyThresholdMS = &value
			}
			for _, field := range []struct {
				name  string
				value int64
				dest  **int64
			}{
				{"failure-threshold", failureThreshold, &options.FailureThreshold},
				{"min-calls", minCalls, &options.MinCalls},
				{"failure-rate", failureRate, &options.FailureRatePct},
				{"half-open-max-probes", maxProbes, &options.HalfOpenMaxProbes},
				{"half-open-success-threshold", successThreshold, &options.HalfOpenSuccessThreshold},
			} {
				if command.Flags().Changed(field.name) {
					if field.value <= 0 {
						return errors.New(field.name + " must be greater than zero")
					}
					value := field.value
					*field.dest = &value
				}
			}
			if options.FailureRatePct != nil && *options.FailureRatePct > 100 {
				return errors.New("failure-rate must not exceed 100")
			}
			return runNetworkCommand(command, dependencies, "open circuit", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				opener, err := requireClientCapability[circuitOpener](client, "circuit changes")
				if err != nil {
					return nil, err
				}
				result, err := opener.CircuitOpenWithOptions(ctx, args[0], options)
				return outputcontract.Circuit(&result), err
			})
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "confirm the circuit change")
	command.Flags().DurationVar(&openFor, "open-for", 0, "time before entering half-open state")
	command.Flags().Int64Var(&failureThreshold, "failure-threshold", 0, "failures that open the circuit")
	command.Flags().DurationVar(&window, "window", 0, "failure measurement window")
	command.Flags().Int64Var(&minCalls, "min-calls", 0, "minimum calls before rate evaluation")
	command.Flags().Int64Var(&failureRate, "failure-rate", 0, "failure percentage that opens the circuit")
	command.Flags().DurationVar(&latency, "latency-threshold", 0, "latency treated as failure")
	command.Flags().StringSliceVar(&errorClasses, "error-class", nil, "error class counted as failure; repeatable")
	command.Flags().Int64Var(&maxProbes, "half-open-max-probes", 0, "maximum concurrent half-open probes")
	command.Flags().Int64Var(&successThreshold, "half-open-success-threshold", 0, "successes needed to close")
	return command
}

type circuitCloser interface {
	CircuitClose(context.Context, string, *int64) (ferricstore.CircuitBreakerStatus, error)
}

func newCircuitCloseCommand(dependencies dependencies) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:     "close <scope>",
		Short:   "Close a circuit breaker",
		Long:    "Close a circuit and permit traffic. This safety-sensitive change requires --yes.",
		Example: "  ferric workflow governance circuit close payments --yes",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !yes {
				return errors.New("closing a circuit requires --yes")
			}
			return runNetworkCommand(command, dependencies, "close circuit", func(ctx context.Context, client connection.Client, _ profile.Profile) (any, error) {
				closer, err := requireClientCapability[circuitCloser](client, "circuit changes")
				if err != nil {
					return nil, err
				}
				result, err := closer.CircuitClose(ctx, args[0], nil)
				return outputcontract.Circuit(&result), err
			})
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "confirm the circuit change")
	return command
}
