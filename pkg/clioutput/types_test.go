package clioutput

import (
	"encoding/json"
	"testing"

	"github.com/samleeney/flows/pkg/model"
)

func TestFlowInspectionUsesStableOrderedDTOs(t *testing.T) {
	flow := &model.Flow{
		Name:           "Inspect",
		Description:    "DTO test",
		ExternalInputs: []string{"source"},
		Defaults:       model.Defaults{PromptExecutor: "codex_cli", Model: "gpt-test", Temperature: 0.2},
		Agents: []model.Agent{{
			Name:     "worker",
			NodeType: model.FunctionNode,
			Language: "python",
			Position: [2]int{3, 4},
			Inputs: map[string]model.Input{
				"zeta":   {From: "seed"},
				"source": {From: "seed", Fallback: "external"},
			},
			Start:   []model.Condition{{When: model.StringOrList{"seed"}, Contains: "ready", MaxRuns: 2}},
			Goal:    &model.Goal{Objective: "Finish", Validation: []string{"It works"}, MaxTurns: 2},
			Content: "output = source",
		}},
	}

	got := FlowInspection("flow.md", flow)
	if len(got.Blocks) != 1 || got.Blocks[0].Kind != "function" || got.Blocks[0].Language != "python" {
		t.Fatalf("block DTO = %#v", got.Blocks)
	}
	if got.Blocks[0].Inputs[0].Name != "source" || got.Blocks[0].Inputs[1].Name != "zeta" {
		t.Fatalf("inputs are not sorted: %#v", got.Blocks[0].Inputs)
	}
	if got.Blocks[0].Goal == nil || got.Blocks[0].Goal.Objective != "Finish" {
		t.Fatalf("goal DTO = %#v", got.Blocks[0].Goal)
	}
	if len(got.Edges) != 4 {
		t.Fatalf("edges = %#v, want input, fallback, input, and start edges", got.Edges)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" {
		t.Fatal("empty JSON")
	}
}
