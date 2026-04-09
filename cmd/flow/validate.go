package main

import (
	"fmt"

	"github.com/samleeney/flows/pkg/parser"
	"github.com/samleeney/flows/pkg/validator"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate a flow file without executing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flow, err := parser.ParseFile(args[0])
			if err != nil {
				return fmt.Errorf("parse error: %w", err)
			}

			if err := validator.Validate(flow); err != nil {
				return fmt.Errorf("validation failed:\n%w", err)
			}

			fmt.Printf("Flow %q is valid (%d agents)\n", flow.Name, len(flow.Agents))
			return nil
		},
	}
}
