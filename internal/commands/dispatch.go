package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/orchestration"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

type dispatchOptions struct {
	TaskRepo              string
	ExecutionRepo         string
	Thread                string
	Lane                  string
	AgentProvider         string
	Operation             string
	Model                 string
	Effort                string
	AcceptProviderDefault bool
	Auto                  bool
	AcknowledgeDeprecated bool
	MCP                   string
	Report                string
	Verify                string
	VerifyName            string
	VerifyTimeoutSeconds  int
	VerifyStrength        string
	Prepare               string
	ProviderArgs          []string
	TimeoutSeconds        int
	Detach                bool
}

type dispatchLifecycle interface {
	Dispatch(context.Context, orchestration.DispatchOptions) (orchestration.DispatchHandle, string, error)
	Status(context.Context, string) (orchestration.DispatchStatus, error)
	Resume(context.Context, string) (orchestration.DispatchReceipt, string, error)
	Receipt(string) (orchestration.DispatchReceipt, error)
}

type dispatchServiceFactory func() (dispatchLifecycle, error)

type dispatchOutput struct {
	DispatchPath string                       `json:"dispatch_path"`
	Dispatch     orchestration.DispatchHandle `json:"dispatch"`
}

type dispatchReceiptOutput struct {
	ReceiptPath string                        `json:"receipt_path"`
	Receipt     orchestration.DispatchReceipt `json:"receipt"`
}

// NewDispatchCmd creates the opt-in orchestration dispatch command.
func NewDispatchCmd() *cobra.Command {
	return newDispatchCmd(newConfiguredDispatchService)
}

func newDispatchCmd(factory dispatchServiceFactory) *cobra.Command {
	opts := defaultDispatchOptions()
	cmd := &cobra.Command{
		Use:   "dispatch <task:<id>|change:<id>>",
		Short: "Run one bounded target with a configured orchestration provider",
		Long: `Freeze one exact ds apply target and run it through the explicitly
configured orchestration provider.

Dispatch waits for provider completion and writes a normalized receipt by
default. Use --detach to return after startup, then inspect or continue the run
with ds dispatch status or ds dispatch resume. Providers cannot checkpoint,
promote, or otherwise mutate the DevSpecs task lifecycle.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()
			err := runDispatch(cmd, factory, opts, args[0])
			telemetry.RecordCommand("dispatch", err == nil, time.Since(start), map[string]any{
				"action": "run", "detach": opts.Detach, "thread": opts.Thread != "",
			})
			return err
		},
	}
	bindDispatchFlags(cmd, &opts)
	cmd.AddCommand(newDispatchStatusCmd(factory))
	cmd.AddCommand(newDispatchResumeCmd(factory))
	cmd.AddCommand(newDispatchReceiptCmd(factory))
	return cmd
}

func runDispatch(cmd *cobra.Command, factory dispatchServiceFactory, opts dispatchOptions, target string) error {
	service, err := factory()
	if err != nil {
		return err
	}
	handle, path, err := service.Dispatch(cmd.Context(), opts.serviceOptions(target))
	if err != nil {
		return err
	}
	if opts.Detach {
		return writeDispatchJSON(cmd.OutOrStdout(), dispatchOutput{DispatchPath: path, Dispatch: handle})
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Dispatch %s: provider run started; waiting for completion\n", handle.DispatchID)
	receipt, receiptPath, err := service.Resume(cmd.Context(), handle.DispatchID)
	if err != nil {
		return err
	}
	return writeDispatchJSON(cmd.OutOrStdout(), dispatchReceiptOutput{ReceiptPath: receiptPath, Receipt: receipt})
}

func newDispatchStatusCmd(factory dispatchServiceFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "status <dispatch-id>",
		Short: "Show provider-neutral state for one dispatch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()
			service, err := factory()
			if err == nil {
				var status orchestration.DispatchStatus
				status, err = service.Status(cmd.Context(), args[0])
				if err == nil {
					err = writeDispatchJSON(cmd.OutOrStdout(), status)
				}
			}
			telemetry.RecordCommand("dispatch", err == nil, time.Since(start), map[string]any{"action": "status"})
			return err
		},
	}
}

func newDispatchResumeCmd(factory dispatchServiceFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "resume <dispatch-id>",
		Short: "Resume monitoring and finalize one dispatch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()
			service, err := factory()
			var output dispatchReceiptOutput
			if err == nil {
				output.Receipt, output.ReceiptPath, err = service.Resume(cmd.Context(), args[0])
				if err == nil {
					err = writeDispatchJSON(cmd.OutOrStdout(), output)
				}
			}
			telemetry.RecordCommand("dispatch", err == nil, time.Since(start), map[string]any{"action": "resume"})
			return err
		},
	}
}

func newDispatchReceiptCmd(factory dispatchServiceFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "receipt <dispatch-id|receipt-path>",
		Short: "Print one bounded normalized dispatch receipt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()
			service, err := factory()
			if err == nil {
				var receipt orchestration.DispatchReceipt
				receipt, err = service.Receipt(args[0])
				if err == nil {
					err = writeDispatchJSON(cmd.OutOrStdout(), receipt)
				}
			}
			telemetry.RecordCommand("dispatch", err == nil, time.Since(start), map[string]any{"action": "receipt"})
			return err
		},
	}
}

func newConfiguredDispatchService() (dispatchLifecycle, error) {
	home, err := config.HomeDir()
	if err != nil {
		return nil, err
	}
	return orchestration.NewService(home), nil
}

func defaultDispatchOptions() dispatchOptions {
	return dispatchOptions{
		TaskRepo: ".", ExecutionRepo: ".", MCP: "auto", VerifyName: "devspecs",
		VerifyTimeoutSeconds: 600, TimeoutSeconds: 600,
	}
}

func bindDispatchFlags(cmd *cobra.Command, opts *dispatchOptions) {
	cmd.Flags().StringVar(&opts.TaskRepo, "task-repo", opts.TaskRepo, "Repository or local planning workspace that owns the target")
	cmd.Flags().StringVar(&opts.ExecutionRepo, "cwd", opts.ExecutionRepo, "Clean Git repository in which the provider should work")
	cmd.Flags().StringVar(&opts.Thread, "thread", "", "Named DevSpecs execution thread")
	cmd.Flags().StringVar(&opts.Lane, "lane", "", "Provider run name; defaults to the dispatch ID")
	cmd.Flags().StringVar(&opts.AgentProvider, "agent-provider", "", "Agent provider passed to the orchestrator")
	cmd.Flags().StringVar(&opts.Operation, "op", "", "Provider operating-point ID")
	cmd.Flags().StringVar(&opts.Model, "model", "", "Agent model override")
	cmd.Flags().StringVar(&opts.Effort, "effort", "", "Agent reasoning-effort override")
	cmd.Flags().BoolVar(&opts.AcceptProviderDefault, "accept-provider-default", false, "Explicitly accept the agent provider default")
	cmd.Flags().BoolVar(&opts.Auto, "auto", false, "Allow provider operating-point fallback")
	cmd.Flags().BoolVar(&opts.AcknowledgeDeprecated, "ack-deprecated", false, "Allow an explicitly acknowledged deprecated fallback")
	cmd.Flags().StringVar(&opts.MCP, "mcp", opts.MCP, "Provider MCP policy")
	cmd.Flags().StringVar(&opts.Report, "report", "", "Required provider report path")
	cmd.Flags().StringVar(&opts.Verify, "verify", "", "Provider verification command")
	cmd.Flags().StringVar(&opts.VerifyName, "verify-name", opts.VerifyName, "Verification label")
	cmd.Flags().IntVar(&opts.VerifyTimeoutSeconds, "verify-timeout", opts.VerifyTimeoutSeconds, "Verification timeout in seconds")
	cmd.Flags().StringVar(&opts.VerifyStrength, "verify-strength", "", "Declared verification strength: suite or smoke")
	cmd.Flags().StringVar(&opts.Prepare, "prepare", "", "Setup command to run before verification")
	cmd.Flags().StringArrayVar(&opts.ProviderArgs, "provider-arg", nil, "Extra agent-provider flag; may be repeated")
	cmd.Flags().IntVar(&opts.TimeoutSeconds, "timeout", opts.TimeoutSeconds, "Provider execution timeout in seconds")
	cmd.Flags().BoolVar(&opts.Detach, "detach", false, "Return after provider startup instead of monitoring to a receipt")
}

func (o dispatchOptions) serviceOptions(target string) orchestration.DispatchOptions {
	return orchestration.DispatchOptions{
		TaskRepo: o.TaskRepo, Target: target, Thread: o.Thread,
		ExecutionRepo: o.ExecutionRepo, Lane: o.Lane,
		AgentProvider: o.AgentProvider, Operation: o.Operation,
		Model: o.Model, Effort: o.Effort,
		AcceptProviderDefault: o.AcceptProviderDefault, Auto: o.Auto,
		AcknowledgeDeprecated: o.AcknowledgeDeprecated, MCP: o.MCP,
		Report: o.Report, Verify: o.Verify, VerifyName: o.VerifyName,
		VerifyTimeoutSeconds: o.VerifyTimeoutSeconds, VerifyStrength: o.VerifyStrength,
		Prepare: o.Prepare, ProviderArgs: o.ProviderArgs,
		TimeoutSeconds: o.TimeoutSeconds,
	}
}

func writeDispatchJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
