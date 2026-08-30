package main

import (
	"fmt"
	"io"
	"os"

	"github.com/samleeney/flows/pkg/clioutput"
	"github.com/samleeney/flows/pkg/model"
	"github.com/samleeney/flows/pkg/parser"
	"github.com/samleeney/flows/pkg/validator"
	"github.com/spf13/cobra"
)

func loadFlowSource(cmd *cobra.Command, path string) ([]byte, string, error) {
	if path == "-" {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, "<stdin>", fmt.Errorf("read <stdin>: %w", err)
		}
		return data, "<stdin>", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, path, fmt.Errorf("read %s: %w", path, err)
	}
	return data, path, nil
}

func parseAndValidate(cmd *cobra.Command, path string) (*model.Flow, string, []clioutput.Diagnostic, error) {
	flow, identity, diagnostics, err := parseFlowSource(cmd, path)
	if err != nil {
		return nil, identity, diagnostics, err
	}
	if err := validator.Validate(flow); err != nil {
		diagnostics := validationDiagnostics(identity, err)
		return flow, identity, diagnostics, err
	}
	return flow, identity, nil, nil
}

func parseFlowSource(cmd *cobra.Command, path string) (*model.Flow, string, []clioutput.Diagnostic, error) {
	source, identity, err := loadFlowSource(cmd, path)
	if err != nil {
		return nil, identity, []clioutput.Diagnostic{{Phase: "parse", Source: identity, Message: err.Error()}}, err
	}
	flow, err := parser.Parse(source)
	if err != nil {
		return nil, identity, []clioutput.Diagnostic{{Phase: "parse", Source: identity, Message: err.Error()}}, err
	}
	return flow, identity, nil, nil
}

func validationDiagnostics(source string, err error) []clioutput.Diagnostic {
	errs, ok := err.(validator.ValidationErrors)
	if !ok {
		return []clioutput.Diagnostic{{Phase: "validation", Source: source, Message: err.Error()}}
	}
	diagnostics := make([]clioutput.Diagnostic, 0, len(errs))
	for _, item := range errs {
		diagnostics = append(diagnostics, clioutput.Diagnostic{
			Phase:   "validation",
			Source:  source,
			Block:   item.Agent,
			Field:   item.Field,
			Message: item.Message,
		})
	}
	return diagnostics
}
