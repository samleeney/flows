package validator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/samleeney/flows/pkg/model"
	"github.com/samleeney/flows/pkg/parser"
)

func TestValidateCodeReviewFlow(t *testing.T) {
	flow, err := parser.ParseFile(filepath.Join("..", "parser", "testdata", "code_review.md"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if err := Validate(flow); err != nil {
		t.Errorf("expected valid flow, got: %v", err)
	}
}

func TestValidateFunctionNodeFlow(t *testing.T) {
	flow, err := parser.ParseFile(filepath.Join("..", "parser", "testdata", "function_node.md"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if err := Validate(flow); err != nil {
		t.Errorf("expected valid flow, got: %v", err)
	}
}

func TestValidateUnknownAgentRef(t *testing.T) {
	flow := &model.Flow{
		Name:           "Test",
		ExternalInputs: []string{"data"},
		Agents: []model.Agent{
			{
				Name:     "worker",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"data": {From: "nonexistent"}},
				Start:    []model.Condition{{Always: &model.AlwaysCondition{MaxRuns: 1}}},
				Content:  "Do work.",
			},
		},
	}

	err := Validate(flow)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention unknown agent: %v", err)
	}
}

func TestValidateInvalidAgentName(t *testing.T) {
	flow := &model.Flow{
		Name:           "Test",
		ExternalInputs: []string{},
		Agents: []model.Agent{
			{
				Name:     "Invalid-Name",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{},
				Start:    []model.Condition{{Always: &model.AlwaysCondition{MaxRuns: 1}}},
				Content:  "Do work.",
			},
		},
	}

	err := Validate(flow)
	if err == nil {
		t.Fatal("expected validation error for invalid name")
	}
	if !strings.Contains(err.Error(), "lowercase") {
		t.Errorf("error should mention naming: %v", err)
	}
}

func TestValidateMissingStartCondition(t *testing.T) {
	flow := &model.Flow{
		Name:           "Test",
		ExternalInputs: []string{"data"},
		Agents: []model.Agent{
			{
				Name:     "worker",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"data": {From: "external"}},
				Start:    []model.Condition{},
				Content:  "Do work.",
			},
		},
	}

	err := Validate(flow)
	if err == nil {
		t.Fatal("expected validation error for missing start")
	}
	if !strings.Contains(err.Error(), "start") {
		t.Errorf("error should mention start: %v", err)
	}
}

func TestValidateCycleWithoutMaxRuns(t *testing.T) {
	flow := &model.Flow{
		Name:           "Test",
		ExternalInputs: []string{"data"},
		Agents: []model.Agent{
			{
				Name:     "agent_a",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"data": {From: "agent_b"}},
				Start:    []model.Condition{{When: model.StringOrList{"agent_b"}}},
				Content:  "Do A.",
			},
			{
				Name:     "agent_b",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"data": {From: "agent_a"}},
				Start:    []model.Condition{{When: model.StringOrList{"agent_a"}}},
				Content:  "Do B.",
			},
		},
	}

	err := Validate(flow)
	if err == nil {
		t.Fatal("expected validation error for cycle without max_runs")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention cycle: %v", err)
	}
}

func TestValidateCycleWithMaxRuns(t *testing.T) {
	flow := &model.Flow{
		Name:           "Test",
		ExternalInputs: []string{"data"},
		Agents: []model.Agent{
			{
				Name:     "agent_a",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"data": {From: "agent_b", Fallback: "external"}},
				Start: []model.Condition{
					{Always: &model.AlwaysCondition{MaxRuns: 1}},
					{When: model.StringOrList{"agent_b"}, MaxRuns: 5},
				},
				Content: "Do A.",
			},
			{
				Name:     "agent_b",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"data": {From: "agent_a"}},
				Start:    []model.Condition{{When: model.StringOrList{"agent_a"}}},
				Content:  "Do B.",
			},
		},
	}

	err := Validate(flow)
	if err != nil {
		t.Errorf("expected valid flow (cycle has max_runs), got: %v", err)
	}
}

func TestValidateMissingContent(t *testing.T) {
	flow := &model.Flow{
		Name:           "Test",
		ExternalInputs: []string{},
		Agents: []model.Agent{
			{
				Name:     "empty",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{},
				Start:    []model.Condition{{Always: &model.AlwaysCondition{MaxRuns: 1}}},
				Content:  "",
			},
		},
	}

	err := Validate(flow)
	if err == nil {
		t.Fatal("expected validation error for missing content")
	}
}
