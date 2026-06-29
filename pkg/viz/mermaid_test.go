package viz

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/samleeney/flows/pkg/model"
	"github.com/samleeney/flows/pkg/parser"
)

func TestMermaidCodeReviewFlow(t *testing.T) {
	flow, err := parser.ParseFile(filepath.Join("..", "parser", "testdata", "code_review.md"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	output := Mermaid(flow)

	// Should start with graph directive
	if !strings.HasPrefix(output, "graph LR\n") {
		t.Error("should start with 'graph LR'")
	}

	// Should contain all agent nodes
	if !strings.Contains(output, "reviewer[reviewer]") {
		t.Error("missing reviewer node")
	}
	if !strings.Contains(output, "fixer[fixer]") {
		t.Error("missing fixer node")
	}
	if !strings.Contains(output, "merger[merger]") {
		t.Error("missing merger node")
	}

	// Should have edges with conditions
	if !strings.Contains(output, `contains "needs_changes"`) {
		t.Error("missing needs_changes condition label")
	}
	if !strings.Contains(output, `contains "approved"`) {
		t.Error("missing approved condition label")
	}

	// Should have edge from fixer to reviewer (loop)
	if !strings.Contains(output, "fixer -->") {
		t.Error("missing fixer -> reviewer edge")
	}

	t.Logf("Output:\n%s", output)
}

func TestMermaidFunctionNode(t *testing.T) {
	flow, err := parser.ParseFile(filepath.Join("..", "parser", "testdata", "function_node.md"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	output := Mermaid(flow)

	// Function nodes should use stadium shape
	if !strings.Contains(output, "processor[[processor [python]]]") {
		t.Errorf("function node should use [[ ]] shape, got:\n%s", output)
	}

	// Prompt nodes should use rectangle
	if !strings.Contains(output, "reporter[reporter]") {
		t.Error("missing reporter node")
	}

	t.Logf("Output:\n%s", output)
}

func TestMermaidMinimal(t *testing.T) {
	flow := &model.Flow{
		Name: "Minimal",
		Agents: []model.Agent{
			{
				Name:     "solo",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{},
				Start:    []model.Condition{{Always: &model.AlwaysCondition{MaxRuns: 1}}},
				Content:  "Do work.",
			},
		},
	}

	output := Mermaid(flow)
	if !strings.Contains(output, "solo[solo]") {
		t.Errorf("missing solo node in:\n%s", output)
	}
}

func TestMermaidExhaustionRoute(t *testing.T) {
	flow := &model.Flow{
		Name: "ExhaustionRoute",
		Agents: []model.Agent{
			{
				Name:     "worker",
				NodeType: model.PromptNode,
				Start: []model.Condition{
					{When: model.StringOrList{"retry"}, MaxRuns: 2, OnExhaustion: "escalate"},
				},
				Content: "Do work.",
			},
			{
				Name:     "retry",
				NodeType: model.PromptNode,
				Start:    []model.Condition{{When: model.StringOrList{"worker"}}},
				Content:  "Retry.",
			},
			{
				Name:     "escalate",
				NodeType: model.PromptNode,
				Content:  "Escalate.",
			},
		},
	}

	output := Mermaid(flow)
	if !strings.Contains(output, "worker -->|on exhaustion| escalate") {
		t.Errorf("missing exhaustion route edge in:\n%s", output)
	}
}
