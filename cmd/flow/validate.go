package main

import (
	"fmt"

	"github.com/samleeney/flows/pkg/clioutput"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate a flow file without executing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flow, source, diagnostics, err := parseAndValidate(cmd, args[0])
			if err != nil {
				if jsonOutput {
					phase := diagnostics[0].Phase
					return failJSON(cmd, "validate", "invalid_flow", fmt.Sprintf("%s failed for %s", phase, source), diagnostics, exitInvalid)
				}
				if diagnostics[0].Phase == "parse" {
					if source == "<stdin>" {
						return fmt.Errorf("parse error (%s): %w", source, err)
					}
					return fmt.Errorf("parse error: %w", err)
				}
				return fmt.Errorf("validation failed:\n%w", err)
			}
			if jsonOutput {
				summary := clioutput.FlowSummary(source, flow)
				return writeJSON(cmd, clioutput.Response{SchemaVersion: clioutput.SchemaVersion, Command: "validate", OK: true, Flow: &summary})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Flow %q is valid (%d agents)\n", flow.Name, len(flow.Agents))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit one versioned JSON result")
	return cmd
}
