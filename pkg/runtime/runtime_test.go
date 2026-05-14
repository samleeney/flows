package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/samleeney/flows/pkg/model"
)

// mockPromptExecutor returns canned responses based on agent content/inputs.
type mockPromptExecutor struct {
	mu        sync.Mutex
	calls     []mockCall
	responder func(content string, inputs map[string]string) string
}

type mockCall struct {
	Content string
	Inputs  map[string]string
}

func (m *mockPromptExecutor) Execute(_ context.Context, content string, inputs map[string]string) (string, error) {
	m.mu.Lock()
	m.calls = append(m.calls, mockCall{Content: content, Inputs: inputs})
	m.mu.Unlock()

	if m.responder != nil {
		return m.responder(content, inputs), nil
	}
	return "mock output", nil
}

type agentAwareMockExecutor struct {
	req ExecutionRequest
}

func (m *agentAwareMockExecutor) Execute(_ context.Context, content string, inputs map[string]string) (string, error) {
	return "fallback", nil
}

func (m *agentAwareMockExecutor) ExecuteAgent(_ context.Context, req ExecutionRequest) (string, error) {
	m.req = req
	return "agent-aware output", nil
}

func TestRunSimpleSequence(t *testing.T) {
	flow := &model.Flow{
		Name:           "Simple",
		ExternalInputs: []string{"data"},
		Agents: []model.Agent{
			{
				Name:     "step_one",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"data": {From: "external"}},
				Start:    []model.Condition{{Always: &model.AlwaysCondition{MaxRuns: 1}}},
				Content:  "Process the data.",
			},
			{
				Name:     "step_two",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"result": {From: "step_one"}},
				Start:    []model.Condition{{When: model.StringOrList{"step_one"}}},
				Content:  "Summarise results.",
			},
		},
	}

	mock := &mockPromptExecutor{
		responder: func(content string, inputs map[string]string) string {
			if strings.Contains(content, "Process") {
				return "processed data"
			}
			return "summary of: " + inputs["result"]
		},
	}

	registry := NewExecutorRegistry(mock)
	result, err := Run(context.Background(), flow, registry, RunOptions{
		ExternalInputs: map[string]string{"data": "raw input"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if result.Outputs["step_one"] != "processed data" {
		t.Errorf("step_one output = %q, want %q", result.Outputs["step_one"], "processed data")
	}
	if result.Outputs["step_two"] != "summary of: processed data" {
		t.Errorf("step_two output = %q, want %q", result.Outputs["step_two"], "summary of: processed data")
	}
}

func TestRunPassesAgentConfigToAgentExecutor(t *testing.T) {
	flow := &model.Flow{
		Name:           "Configured",
		ExternalInputs: []string{"data"},
		Defaults:       model.Defaults{Model: "default-model", Temperature: 0.2},
		Agents: []model.Agent{
			{
				Name:        "reviewer",
				NodeType:    model.PromptNode,
				Inputs:      map[string]model.Input{"data": {From: "external"}},
				Start:       []model.Condition{{Always: &model.AlwaysCondition{MaxRuns: 1}}},
				Content:     "Review the data.",
				Model:       "agent-model",
				Temperature: 0.7,
			},
		},
	}

	mock := &agentAwareMockExecutor{}
	result, err := Run(context.Background(), flow, NewExecutorRegistry(mock), RunOptions{
		ExternalInputs: map[string]string{"data": "payload"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Outputs["reviewer"] != "agent-aware output" {
		t.Fatalf("output = %q, want agent-aware output", result.Outputs["reviewer"])
	}
	if mock.req.FlowName != "Configured" {
		t.Fatalf("FlowName = %q, want Configured", mock.req.FlowName)
	}
	if mock.req.Defaults.Model != "default-model" {
		t.Fatalf("Defaults.Model = %q, want default-model", mock.req.Defaults.Model)
	}
	if mock.req.Agent.Model != "agent-model" {
		t.Fatalf("Agent.Model = %q, want agent-model", mock.req.Agent.Model)
	}
	if mock.req.Inputs["data"] != "payload" {
		t.Fatalf("input data = %q, want payload", mock.req.Inputs["data"])
	}
}

func TestRunConditionalBranch(t *testing.T) {
	flow := &model.Flow{
		Name:           "Branch",
		ExternalInputs: []string{"code"},
		Agents: []model.Agent{
			{
				Name:     "reviewer",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"code": {From: "external"}},
				Start:    []model.Condition{{Always: &model.AlwaysCondition{MaxRuns: 1}}},
				Content:  "Review code.",
			},
			{
				Name:     "fixer",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"feedback": {From: "reviewer"}},
				Start:    []model.Condition{{When: model.StringOrList{"reviewer"}, Contains: "needs_changes"}},
				Content:  "Fix issues.",
			},
			{
				Name:     "merger",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"code": {From: "external"}},
				Start:    []model.Condition{{When: model.StringOrList{"reviewer"}, Contains: "approved"}},
				Content:  "Merge code.",
			},
		},
	}

	mock := &mockPromptExecutor{
		responder: func(content string, inputs map[string]string) string {
			if strings.Contains(content, "Review") {
				return "approved"
			}
			if strings.Contains(content, "Merge") {
				return "merged"
			}
			return "fixed"
		},
	}

	registry := NewExecutorRegistry(mock)
	result, err := Run(context.Background(), flow, registry, RunOptions{
		ExternalInputs: map[string]string{"code": "x = 1"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Reviewer said "approved", so merger should fire but fixer should not
	if _, ok := result.Outputs["merger"]; !ok {
		t.Error("merger should have fired")
	}
	if _, ok := result.Outputs["fixer"]; ok {
		t.Error("fixer should NOT have fired (reviewer said approved)")
	}
}

func TestRunLoop(t *testing.T) {
	callCount := 0
	flow := &model.Flow{
		Name:           "Loop",
		ExternalInputs: []string{"code"},
		Agents: []model.Agent{
			{
				Name:     "reviewer",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"code": {From: "fixer", Fallback: "external"}},
				Start: []model.Condition{
					{Always: &model.AlwaysCondition{MaxRuns: 1}},
					{When: model.StringOrList{"fixer"}, MaxRuns: 3},
				},
				Content: "Review code.",
			},
			{
				Name:     "fixer",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"feedback": {From: "reviewer"}},
				Start:    []model.Condition{{When: model.StringOrList{"reviewer"}, Contains: "needs_changes"}},
				Content:  "Fix issues.",
			},
		},
	}

	mock := &mockPromptExecutor{
		responder: func(content string, inputs map[string]string) string {
			if strings.Contains(content, "Review") {
				callCount++
				if callCount >= 3 {
					return "approved"
				}
				return "needs_changes: fix line 5"
			}
			return "fixed code"
		},
	}

	registry := NewExecutorRegistry(mock)
	result, err := Run(context.Background(), flow, registry, RunOptions{
		ExternalInputs: map[string]string{"code": "buggy code"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if callCount != 3 {
		t.Errorf("reviewer should have run 3 times, ran %d", callCount)
	}
	if !strings.Contains(result.Outputs["reviewer"], "approved") {
		t.Errorf("final reviewer output should contain approved, got %q", result.Outputs["reviewer"])
	}
}

func TestRunParallelExecution(t *testing.T) {
	flow := &model.Flow{
		Name:           "Parallel",
		ExternalInputs: []string{"data"},
		Agents: []model.Agent{
			{
				Name:     "worker_a",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"data": {From: "external"}},
				Start:    []model.Condition{{Always: &model.AlwaysCondition{MaxRuns: 1}}},
				Content:  "Work A.",
			},
			{
				Name:     "worker_b",
				NodeType: model.PromptNode,
				Inputs:   map[string]model.Input{"data": {From: "external"}},
				Start:    []model.Condition{{Always: &model.AlwaysCondition{MaxRuns: 1}}},
				Content:  "Work B.",
			},
			{
				Name:     "joiner",
				NodeType: model.PromptNode,
				Inputs: map[string]model.Input{
					"a": {From: "worker_a"},
					"b": {From: "worker_b"},
				},
				Start:   []model.Condition{{When: model.StringOrList{"worker_a", "worker_b"}}},
				Content: "Join results.",
			},
		},
	}

	mock := &mockPromptExecutor{
		responder: func(content string, inputs map[string]string) string {
			return fmt.Sprintf("output from: %s", content)
		},
	}

	registry := NewExecutorRegistry(mock)
	result, err := Run(context.Background(), flow, registry, RunOptions{
		ExternalInputs: map[string]string{"data": "input"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if _, ok := result.Outputs["worker_a"]; !ok {
		t.Error("worker_a should have output")
	}
	if _, ok := result.Outputs["worker_b"]; !ok {
		t.Error("worker_b should have output")
	}
	if _, ok := result.Outputs["joiner"]; !ok {
		t.Error("joiner should have output")
	}
}

func TestRunBashFunctionNode(t *testing.T) {
	flow := &model.Flow{
		Name:           "BashFunc",
		ExternalInputs: []string{"name"},
		Agents: []model.Agent{
			{
				Name:     "greeter",
				NodeType: model.FunctionNode,
				Language: "bash",
				Inputs:   map[string]model.Input{"name": {From: "external"}},
				Start:    []model.Condition{{Always: &model.AlwaysCondition{MaxRuns: 1}}},
				Content:  `echo "hello world"`,
			},
		},
	}

	registry := NewExecutorRegistry(
		&mockPromptExecutor{},
		&BashExecutor{},
	)

	result, err := Run(context.Background(), flow, registry, RunOptions{
		ExternalInputs: map[string]string{"name": "flow"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if result.Outputs["greeter"] != "hello world" {
		t.Errorf("greeter output = %q, want %q", result.Outputs["greeter"], "hello world")
	}
}
