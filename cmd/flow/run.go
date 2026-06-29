package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/samleeney/flows/pkg/live"
	"github.com/samleeney/flows/pkg/model"
	"github.com/samleeney/flows/pkg/parser"
	"github.com/samleeney/flows/pkg/runtime"
	"github.com/samleeney/flows/pkg/validator"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var (
		inputs     []string
		foreground bool
		verbose    bool
		dryRun     bool
		outDir     string

		promptExecutor string
		llmProvider    string
		llmModel       string
		maxTokens      int
		llmTimeout     time.Duration
	)

	cmd := &cobra.Command{
		Use:   "run <file>",
		Short: "Start a flow run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flow, err := parser.ParseFile(args[0])
			if err != nil {
				return fmt.Errorf("parse error: %w", err)
			}

			if err := validator.Validate(flow); err != nil {
				return fmt.Errorf("validation failed:\n%w", err)
			}

			// Parse --input flags
			externalInputs := make(map[string]string)
			externalInputOrigins := make(map[string]live.ExternalInputOrigin)
			for _, kv := range inputs {
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid input format %q, expected name=value", kv)
				}
				name, value := parts[0], parts[1]

				// @filepath means read from file
				if strings.HasPrefix(value, "@") {
					path := value[1:]
					data, err := os.ReadFile(path)
					if err != nil {
						return fmt.Errorf("reading input file for %q: %w", name, err)
					}
					value = string(data)
					externalInputOrigins[name] = externalInputOriginFromFile(name, path, value)
				} else {
					externalInputOrigins[name] = externalInputOriginFromInline(name, value)
				}
				externalInputs[name] = value
			}

			if dryRun {
				fmt.Printf("Flow: %s\n", flow.Name)
				fmt.Printf("Agents: %d\n", len(flow.Agents))
				fmt.Printf("External inputs: %v\n", flow.ExternalInputs)
				fmt.Println("Dry run — not executing.")
				return nil
			}

			if missing := missingExternalInputs(flow, externalInputs); len(missing) > 0 {
				return fmt.Errorf("missing external input(s): %s", strings.Join(missing, ", "))
			}

			canonical, err := live.CanonicalFlowPath(args[0])
			if err != nil {
				return fmt.Errorf("canonicalize: %w", err)
			}
			flowKey := live.FlowKey(canonical)

			editorSession, err := ensureRunEditor(args[0], canonical, flowKey, !foreground)
			if err != nil {
				return err
			}
			defer editorSession.Close()
			fmt.Printf("View flow: %s\n", editorSession.BaseURL)

			if !foreground {
				pid, logPath, err := startBackgroundForegroundRun(flowKey)
				if err != nil {
					return err
				}
				fmt.Printf("Run started in background (pid %d).\n", pid)
				fmt.Printf("Log: %s\n", logPath)
				return nil
			}

			// Build executor registry
			defaultPromptExecutor := promptExecutor
			if defaultPromptExecutor == "" && llmProvider != "" {
				defaultPromptExecutor = llmProvider
			}

			prompt := runtime.NewPromptRouterExecutor(runtime.PromptRouterConfig{
				DefaultExecutor: defaultPromptExecutor,
				HTTP: runtime.HTTPPromptConfig{
					Provider:         llmProvider,
					Model:            llmModel,
					AnthropicAPIKey:  os.Getenv("ANTHROPIC_API_KEY"),
					OpenAIAPIKey:     os.Getenv("OPENAI_API_KEY"),
					AnthropicBaseURL: os.Getenv("ANTHROPIC_BASE_URL"),
					OpenAIBaseURL:    os.Getenv("OPENAI_BASE_URL"),
					AnthropicVersion: os.Getenv("ANTHROPIC_VERSION"),
					MaxTokens:        maxTokens,
					Timeout:          llmTimeout,
				},
				Codex: runtime.CodexCLIConfig{
					Command: os.Getenv("FLOW_CODEX_COMMAND"),
					Model:   llmModel,
					Timeout: llmTimeout,
				},
			})
			registry := runtime.NewExecutorRegistry(prompt, &runtime.BashExecutor{}, &runtime.PythonExecutor{})

			observer := buildLiveObserver(editorSession.Descriptors)
			defer observer.Close()

			opts := runtime.RunOptions{
				ExternalInputs:       externalInputs,
				ExternalInputOrigins: externalInputOrigins,
				Verbose:              verbose,
				FlowKey:              flowKey,
				Observer:             observer,
			}

			if foreground || verbose {
				opts.OnAgentStart = func(name string, iter int) {
					fmt.Printf("[%s] iteration %d starting...\n", name, iter)
				}
				opts.OnAgentDone = func(name string, iter int, output string, err error) {
					if err != nil {
						fmt.Printf("[%s] iteration %d FAILED: %v\n", name, iter, err)
					} else {
						preview := output
						if len(preview) > 100 {
							preview = preview[:100] + "..."
						}
						fmt.Printf("[%s] iteration %d done: %s\n", name, iter, preview)
					}
				}
			}

			result, err := runtime.Run(context.Background(), flow, registry, opts)
			if err != nil {
				return fmt.Errorf("execution failed: %w", err)
			}

			// Write outputs
			if outDir != "" {
				if err := os.MkdirAll(outDir, 0o755); err != nil {
					return err
				}
				for name, output := range result.Outputs {
					path := filepath.Join(outDir, name+".txt")
					if err := os.WriteFile(path, []byte(output), 0o644); err != nil {
						return fmt.Errorf("writing output for %q: %w", name, err)
					}
				}
				fmt.Printf("Outputs written to %s/\n", outDir)
			} else {
				for name, output := range result.Outputs {
					fmt.Printf("=== %s ===\n%s\n\n", name, output)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringArrayVar(&inputs, "input", nil, "External input as name=value (repeatable)")
	cmd.Flags().BoolVarP(&foreground, "foreground", "f", false, "Run in the foreground and print live progress")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Print agent execution details")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and show plan without running")
	cmd.Flags().StringVar(&outDir, "output", "", "Directory to write agent outputs")
	cmd.Flags().StringVar(&promptExecutor, "prompt-executor", os.Getenv("FLOW_PROMPT_EXECUTOR"), "Override prompt executor for all prompt nodes: codex_cli, codex_cli_write, anthropic_api, or openai_api")
	cmd.Flags().StringVar(&llmProvider, "llm-provider", os.Getenv("FLOW_LLM_PROVIDER"), "LLM provider for prompt nodes: anthropic or openai (default: infer from model)")
	cmd.Flags().StringVar(&llmModel, "model", os.Getenv("FLOW_MODEL"), "Override model for all prompt nodes (default: flow/agent model)")
	cmd.Flags().IntVar(&maxTokens, "max-tokens", envInt("FLOW_MAX_TOKENS", runtime.DefaultMaxTokens), "Maximum output tokens for prompt nodes")
	cmd.Flags().DurationVar(&llmTimeout, "llm-timeout", envDuration("FLOW_LLM_TIMEOUT", runtime.DefaultPromptTimeout), "Timeout for each prompt-node LLM request")

	return cmd
}

// buildLiveObserver returns a fan-out observer over discovered editor
// descriptors. If none are present, a NopObserver is returned and live
// reporting is silently disabled.
func buildLiveObserver(descs []live.Descriptor) live.Observer {
	if len(descs) == 0 {
		return live.NopObserver{}
	}
	children := make([]live.Observer, 0, len(descs))
	for _, d := range descs {
		children = append(children, live.NewHTTPObserver(d.BaseURL, d.Token))
	}
	return live.NewFanoutObserver(children...)
}

func externalInputOriginFromInline(name, value string) live.ExternalInputOrigin {
	preview, total, truncated := live.TruncatePreviewUTF8(value, runtime.PreviewMaxBytes)
	return live.ExternalInputOrigin{
		Name:             name,
		Source:           "inline",
		Bytes:            total,
		Preview:          preview,
		PreviewTruncated: truncated,
	}
}

func externalInputOriginFromFile(name, path, value string) live.ExternalInputOrigin {
	preview, total, truncated := live.TruncatePreviewUTF8(value, runtime.PreviewMaxBytes)
	abs, err := filepath.Abs(path)
	if err == nil {
		path = filepath.Clean(abs)
	}
	return live.ExternalInputOrigin{
		Name:             name,
		Source:           "file",
		Path:             path,
		FileName:         filepath.Base(path),
		Bytes:            total,
		Preview:          preview,
		PreviewTruncated: truncated,
	}
}

func missingExternalInputs(flow *model.Flow, inputs map[string]string) []string {
	var missing []string
	for _, name := range flow.ExternalInputs {
		if _, ok := inputs[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envDuration(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
