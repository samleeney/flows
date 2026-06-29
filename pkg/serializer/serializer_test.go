package serializer

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/samleeney/flows/pkg/model"
	"github.com/samleeney/flows/pkg/parser"
)

func TestSerializeCodeReviewFlow(t *testing.T) {
	flow, err := parser.ParseFile(filepath.Join("..", "parser", "testdata", "code_review.md"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	output, err := Serialize(flow)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	if len(output) == 0 {
		t.Fatal("output is empty")
	}

	// Round-trip: parse the serialized output
	flow2, err := parser.Parse(output)
	if err != nil {
		t.Fatalf("round-trip parse: %v\n\nSerialized output:\n%s", err, string(output))
	}

	// Compare flow-level fields
	if flow2.Name != flow.Name {
		t.Errorf("name: got %q, want %q", flow2.Name, flow.Name)
	}
	if flow2.Description != flow.Description {
		t.Errorf("description: got %q, want %q", flow2.Description, flow.Description)
	}
	if len(flow2.ExternalInputs) != len(flow.ExternalInputs) {
		t.Errorf("external_inputs: got %v, want %v", flow2.ExternalInputs, flow.ExternalInputs)
	}
	if flow2.Defaults.Model != flow.Defaults.Model {
		t.Errorf("defaults.model: got %q, want %q", flow2.Defaults.Model, flow.Defaults.Model)
	}

	// Compare agents
	if len(flow2.Agents) != len(flow.Agents) {
		t.Fatalf("agents: got %d, want %d", len(flow2.Agents), len(flow.Agents))
	}

	for i, a := range flow.Agents {
		b := flow2.Agents[i]
		if b.Name != a.Name {
			t.Errorf("agent[%d].name: got %q, want %q", i, b.Name, a.Name)
		}
		if b.NodeType != a.NodeType {
			t.Errorf("agent[%d].nodeType: got %v, want %v", i, b.NodeType, a.NodeType)
		}
		if b.Position != a.Position {
			t.Errorf("agent[%d].position: got %v, want %v", i, b.Position, a.Position)
		}
		if len(b.Inputs) != len(a.Inputs) {
			t.Errorf("agent[%d].inputs: got %d, want %d", i, len(b.Inputs), len(a.Inputs))
		}
		for name, inputA := range a.Inputs {
			inputB, ok := b.Inputs[name]
			if !ok {
				t.Errorf("agent[%d].inputs[%s]: missing", i, name)
				continue
			}
			if inputB.From != inputA.From || inputB.Fallback != inputA.Fallback {
				t.Errorf("agent[%d].inputs[%s]: got %+v, want %+v", i, name, inputB, inputA)
			}
		}
		if len(b.Start) != len(a.Start) {
			t.Errorf("agent[%d].start: got %d conditions, want %d", i, len(b.Start), len(a.Start))
		}
	}
}

func TestSerializeFunctionNode(t *testing.T) {
	flow, err := parser.ParseFile(filepath.Join("..", "parser", "testdata", "function_node.md"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	output, err := Serialize(flow)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	flow2, err := parser.Parse(output)
	if err != nil {
		t.Fatalf("round-trip parse: %v\n\nSerialized output:\n%s", err, string(output))
	}

	if len(flow2.Agents) != 2 {
		t.Fatalf("agents: got %d, want 2", len(flow2.Agents))
	}

	proc := flow2.Agents[0]
	if proc.NodeType != model.FunctionNode {
		t.Errorf("processor.nodeType: got %v, want FunctionNode", proc.NodeType)
	}
	if proc.Language != "python" {
		t.Errorf("processor.language: got %q, want %q", proc.Language, "python")
	}

	reporter := flow2.Agents[1]
	if reporter.NodeType != model.PromptNode {
		t.Errorf("reporter.nodeType: got %v, want PromptNode", reporter.NodeType)
	}
}

func TestSerializeIdempotent(t *testing.T) {
	// Parse → serialize → parse → serialize should produce identical bytes
	// after the first round-trip. This catches parser double-counting bugs
	// where content grows on each round-trip.
	flow, err := parser.ParseFile(filepath.Join("..", "parser", "testdata", "code_review.md"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	first, err := Serialize(flow)
	if err != nil {
		t.Fatalf("first serialize: %v", err)
	}

	flow2, err := parser.Parse(first)
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}

	second, err := Serialize(flow2)
	if err != nil {
		t.Fatalf("second serialize: %v", err)
	}

	if string(first) != string(second) {
		t.Errorf("round-trip not idempotent:\nfirst length: %d\nsecond length: %d", len(first), len(second))
	}

	// Specifically check that the reviewer's prompt appears exactly once
	phrase := "Review the provided code against the guidelines."
	for _, agent := range flow2.Agents {
		if agent.Name == "reviewer" {
			if n := strings.Count(agent.Content, phrase); n != 1 {
				t.Errorf("reviewer content has %q repeated %d times after round-trip", phrase, n)
			}
		}
	}
}

func TestSerializeMinimalFlow(t *testing.T) {
	flow := &model.Flow{
		Name:           "Minimal",
		ExternalInputs: []string{"data"},
		Defaults: model.Defaults{
			PromptExecutor: "codex_cli",
			Model:          "gpt-5.3-codex-spark",
		},
		Agents: []model.Agent{
			{
				Name:           "worker",
				NodeType:       model.PromptNode,
				PromptExecutor: "anthropic_api",
				Inputs:         map[string]model.Input{"data": {From: "external"}},
				Start:          []model.Condition{{Always: &model.AlwaysCondition{MaxRuns: 1}}},
				Content:        "Do the work.",
			},
		},
	}

	output, err := Serialize(flow)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	flow2, err := parser.Parse(output)
	if err != nil {
		t.Fatalf("round-trip parse: %v\n\nSerialized output:\n%s", err, string(output))
	}

	if flow2.Name != "Minimal" {
		t.Errorf("name: got %q, want %q", flow2.Name, "Minimal")
	}
	if flow2.Defaults.PromptExecutor != "codex_cli" {
		t.Errorf("defaults.prompt_executor: got %q, want codex_cli", flow2.Defaults.PromptExecutor)
	}
	if flow2.Defaults.Model != "gpt-5.3-codex-spark" {
		t.Errorf("defaults.model: got %q, want gpt-5.3-codex-spark", flow2.Defaults.Model)
	}
	if len(flow2.Agents) != 1 {
		t.Fatalf("agents: got %d, want 1", len(flow2.Agents))
	}
	if flow2.Agents[0].Name != "worker" {
		t.Errorf("agent name: got %q, want %q", flow2.Agents[0].Name, "worker")
	}
	if flow2.Agents[0].PromptExecutor != "anthropic_api" {
		t.Errorf("agent prompt_executor: got %q, want anthropic_api", flow2.Agents[0].PromptExecutor)
	}
}

func TestSerializeGoalBlock(t *testing.T) {
	flow := &model.Flow{
		Name:           "Goal Flow",
		ExternalInputs: []string{"topic"},
		Agents: []model.Agent{
			{
				Name:     "writer",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"topic": {From: "external"}},
				Start:    []model.Condition{{Always: &model.AlwaysCondition{MaxRuns: 1}}},
				Goal: &model.Goal{
					Objective:    "Write exactly three concise bullets.",
					Validation:   []string{"Exactly three lines.", "Each line starts with '- '."},
					MaxTurns:     2,
					OnExhaustion: "escalate",
				},
				Content: "Write the bullets for the supplied topic.",
			},
		},
	}

	output, err := Serialize(flow)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if !strings.Contains(string(output), "```goal\n") {
		t.Fatalf("serialized output missing goal block:\n%s", string(output))
	}

	flow2, err := parser.Parse(output)
	if err != nil {
		t.Fatalf("round-trip parse: %v\n\nSerialized output:\n%s", err, string(output))
	}
	goal := flow2.Agents[0].Goal
	if goal == nil {
		t.Fatal("goal missing after round-trip")
	}
	if goal.Objective != "Write exactly three concise bullets." {
		t.Fatalf("objective = %q", goal.Objective)
	}
	if len(goal.Validation) != 2 {
		t.Fatalf("validation len = %d, want 2", len(goal.Validation))
	}
	if strings.Contains(flow2.Agents[0].Content, "```goal") {
		t.Fatalf("goal block leaked into content: %q", flow2.Agents[0].Content)
	}
}
