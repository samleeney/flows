package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/samleeney/flows/pkg/clioutput"
	"github.com/samleeney/flows/pkg/live"
	"github.com/spf13/cobra"
)

const (
	exitInternal  = 1
	exitInvalid   = 2
	exitExecution = 3
)

type commandError struct {
	err      error
	exitCode int
	emitted  bool
}

func (e *commandError) Error() string { return e.err.Error() }
func (e *commandError) Unwrap() error { return e.err }

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "flow",
		Short:         "Declarative agent orchestration tool",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newRunCmd(), newValidateCmd(), newInspectCmd(), newVizCmd(), newChartCmd())
	return root
}

func runCLI(args []string, in io.Reader, out, errOut io.Writer) int {
	root := newRootCmd()
	root.SetArgs(args)
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)
	if err := root.Execute(); err != nil {
		var commandErr *commandError
		if errors.As(err, &commandErr) {
			if !commandErr.emitted {
				fmt.Fprintln(errOut, commandErr.err)
			}
			return commandErr.exitCode
		}
		if mode := structuredMode(args); mode != "" {
			command := commandName(args)
			if mode == "jsonl" && command == "run" {
				runID, idErr := live.NewRunID()
				if idErr == nil {
					event := clioutput.Event{
						SchemaVersion: clioutput.SchemaVersion,
						Command:       command,
						OK:            false,
						Event:         "execution_error",
						RunID:         runID,
						Sequence:      1,
						Timestamp:     time.Now().UTC(),
						Error:         &clioutput.Error{Code: "invalid_arguments", Message: err.Error()},
					}
					if encodeErr := json.NewEncoder(out).Encode(event); encodeErr == nil {
						return exitInvalid
					}
				}
			}
			response := clioutput.Response{
				SchemaVersion: clioutput.SchemaVersion,
				Command:       command,
				OK:            false,
				Error:         &clioutput.Error{Code: "invalid_arguments", Message: err.Error()},
			}
			if encodeErr := json.NewEncoder(out).Encode(response); encodeErr == nil {
				return exitInvalid
			}
		}
		fmt.Fprintln(errOut, err)
		return exitInternal
	}
	return 0
}

func structuredMode(args []string) string {
	for _, arg := range args {
		switch arg {
		case "--json", "--json=true":
			return "json"
		case "--jsonl", "--jsonl=true":
			return "jsonl"
		}
	}
	return ""
}

func commandName(args []string) string {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return "flow"
}

func writeJSON(cmd *cobra.Command, response clioutput.Response) error {
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(response); err != nil {
		return &commandError{err: fmt.Errorf("write JSON result: %w", err), exitCode: exitInternal}
	}
	return nil
}

func failJSON(cmd *cobra.Command, command, code, message string, diagnostics []clioutput.Diagnostic, exitCode int) error {
	response := clioutput.Response{
		SchemaVersion: clioutput.SchemaVersion,
		Command:       command,
		OK:            false,
		Error: &clioutput.Error{
			Code:        code,
			Message:     message,
			Diagnostics: diagnostics,
		},
	}
	if err := writeJSON(cmd, response); err != nil {
		return err
	}
	return &commandError{err: errors.New(message), exitCode: exitCode, emitted: true}
}
