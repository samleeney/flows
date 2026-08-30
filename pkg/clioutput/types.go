// Package clioutput defines the stable, versioned wire format emitted by the
// flow CLI. These DTOs intentionally do not expose parser or runtime structs.
package clioutput

import (
	"sort"
	"time"

	"github.com/samleeney/flows/pkg/model"
)

// SchemaVersion is the current machine-facing CLI schema version.
const SchemaVersion = 1

// Response is a single-document result emitted by --json commands.
type Response struct {
	SchemaVersion  int      `json:"schema_version"`
	Command        string   `json:"command"`
	OK             bool     `json:"ok"`
	Flow           *Flow    `json:"flow,omitempty"`
	Mode           string   `json:"mode,omitempty"`
	RequiredInputs []string `json:"required_inputs,omitempty"`
	SuppliedInputs []string `json:"supplied_inputs,omitempty"`
	Run            *Run     `json:"run,omitempty"`
	Mermaid        string   `json:"mermaid,omitempty"`
	Error          *Error   `json:"error,omitempty"`
}

// Error is a stable machine-readable error.
type Error struct {
	Code        string       `json:"code"`
	Message     string       `json:"message"`
	Details     string       `json:"details,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// Diagnostic identifies a parse or validation problem.
type Diagnostic struct {
	Phase   string `json:"phase"`
	Source  string `json:"source,omitempty"`
	Block   string `json:"block,omitempty"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// Flow is the public representation of a parsed flow.
type Flow struct {
	Source         string   `json:"source"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	ExternalInputs []string `json:"external_inputs"`
	BlockCount     int      `json:"block_count"`
	Defaults       Defaults `json:"defaults"`
	Blocks         []Block  `json:"blocks,omitempty"`
	Edges          []Edge   `json:"edges,omitempty"`
}

// Defaults contains flow-wide executor settings.
type Defaults struct {
	PromptExecutor string  `json:"prompt_executor"`
	Model          string  `json:"model"`
	Temperature    float64 `json:"temperature"`
}

// Block is a prompt or deterministic function block in source order.
type Block struct {
	Name           string           `json:"name"`
	Kind           string           `json:"kind"`
	Language       string           `json:"language,omitempty"`
	Position       [2]int           `json:"position"`
	Inputs         []Input          `json:"inputs"`
	Start          []StartCondition `json:"start"`
	Goal           *Goal            `json:"goal,omitempty"`
	PromptExecutor string           `json:"prompt_executor,omitempty"`
	Model          string           `json:"model,omitempty"`
	Temperature    float64          `json:"temperature,omitempty"`
	OnError        string           `json:"on_error,omitempty"`
	OnExhaustion   string           `json:"on_exhaustion,omitempty"`
	Content        string           `json:"content"`
}

// Input is a named block input. Inputs are sorted by name for determinism.
type Input struct {
	Name     string `json:"name"`
	From     string `json:"from"`
	Fallback string `json:"fallback,omitempty"`
}

// StartCondition describes when a block becomes runnable.
type StartCondition struct {
	Always       bool     `json:"always,omitempty"`
	When         []string `json:"when,omitempty"`
	Contains     string   `json:"contains,omitempty"`
	MaxRuns      int      `json:"max_runs,omitempty"`
	OnExhaustion string   `json:"on_exhaustion,omitempty"`
}

// Goal is durable objective metadata associated with a block.
type Goal struct {
	Objective    string   `json:"objective"`
	Validation   []string `json:"validation"`
	MaxTurns     int      `json:"max_turns,omitempty"`
	TokenBudget  int      `json:"token_budget,omitempty"`
	OnExhaustion string   `json:"on_exhaustion,omitempty"`
}

// Edge explicitly represents a data or control dependency.
type Edge struct {
	Kind      string `json:"kind"`
	From      string `json:"from"`
	To        string `json:"to"`
	Input     string `json:"input,omitempty"`
	Contains  string `json:"contains,omitempty"`
	Condition int    `json:"condition,omitempty"`
}

// Output is a final, untruncated block output.
type Output struct {
	Block string `json:"block"`
	Value string `json:"value"`
	Path  string `json:"path,omitempty"`
}

// Run is the terminal execution summary.
type Run struct {
	RunID      string    `json:"run_id"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`
	Outputs    []Output  `json:"outputs"`
	OutputDir  string    `json:"output_directory,omitempty"`
}

// Event is one JSON Lines execution record.
type Event struct {
	SchemaVersion int       `json:"schema_version"`
	Command       string    `json:"command"`
	OK            bool      `json:"ok"`
	Event         string    `json:"event"`
	RunID         string    `json:"run_id"`
	Sequence      uint64    `json:"sequence"`
	Timestamp     time.Time `json:"timestamp"`
	Block         string    `json:"block,omitempty"`
	Iteration     int       `json:"iteration,omitempty"`
	Status        string    `json:"status,omitempty"`
	DurationMS    int64     `json:"duration_ms,omitempty"`
	OutputPreview string    `json:"output_preview,omitempty"`
	Outputs       *[]Output `json:"outputs,omitempty"`
	Error         *Error    `json:"error,omitempty"`
}

// FlowSummary constructs the compact flow representation used by validate
// and run. Use FlowInspection when block-level graph data is required.
func FlowSummary(source string, flow *model.Flow) Flow {
	return Flow{
		Source:         source,
		Name:           flow.Name,
		Description:    flow.Description,
		ExternalInputs: cloneStrings(flow.ExternalInputs),
		BlockCount:     len(flow.Agents),
		Defaults: Defaults{
			PromptExecutor: flow.Defaults.PromptExecutor,
			Model:          flow.Defaults.Model,
			Temperature:    flow.Defaults.Temperature,
		},
	}
}

// FlowInspection constructs the complete public graph representation.
func FlowInspection(source string, flow *model.Flow) Flow {
	out := FlowSummary(source, flow)
	out.Blocks = make([]Block, 0, len(flow.Agents))
	out.Edges = make([]Edge, 0)
	for _, agent := range flow.Agents {
		block := Block{
			Name:           agent.Name,
			Kind:           "prompt",
			Language:       agent.Language,
			Position:       agent.Position,
			Inputs:         make([]Input, 0, len(agent.Inputs)),
			Start:          make([]StartCondition, 0, len(agent.Start)),
			PromptExecutor: agent.PromptExecutor,
			Model:          agent.Model,
			Temperature:    agent.Temperature,
			OnError:        agent.OnError,
			OnExhaustion:   agent.OnExhaustion,
			Content:        agent.Content,
		}
		if agent.NodeType == model.FunctionNode {
			block.Kind = "function"
		}
		inputNames := make([]string, 0, len(agent.Inputs))
		for name := range agent.Inputs {
			inputNames = append(inputNames, name)
		}
		sort.Strings(inputNames)
		for _, name := range inputNames {
			input := agent.Inputs[name]
			block.Inputs = append(block.Inputs, Input{Name: name, From: input.From, Fallback: input.Fallback})
			out.Edges = append(out.Edges, Edge{Kind: "input", From: input.From, To: agent.Name, Input: name})
			if input.Fallback != "" {
				out.Edges = append(out.Edges, Edge{Kind: "fallback", From: input.Fallback, To: agent.Name, Input: name})
			}
		}
		for i, condition := range agent.Start {
			start := StartCondition{
				When:         cloneStrings(condition.When),
				Contains:     condition.Contains,
				MaxRuns:      condition.MaxRuns,
				OnExhaustion: condition.OnExhaustion,
			}
			if condition.Always != nil {
				start.Always = true
				if condition.Always.MaxRuns > 0 {
					start.MaxRuns = condition.Always.MaxRuns
				}
			}
			block.Start = append(block.Start, start)
			for _, dependency := range condition.When {
				out.Edges = append(out.Edges, Edge{Kind: "start", From: dependency, To: agent.Name, Contains: condition.Contains, Condition: i})
			}
			if condition.OnExhaustion != "" && condition.OnExhaustion != "stop" && condition.OnExhaustion != "continue" {
				out.Edges = append(out.Edges, Edge{Kind: "exhaustion", From: agent.Name, To: condition.OnExhaustion, Condition: i})
			}
		}
		if agent.OnExhaustion != "" && agent.OnExhaustion != "stop" && agent.OnExhaustion != "continue" {
			out.Edges = append(out.Edges, Edge{Kind: "exhaustion", From: agent.Name, To: agent.OnExhaustion})
		}
		if agent.Goal != nil {
			block.Goal = &Goal{
				Objective:    agent.Goal.Objective,
				Validation:   cloneStrings(agent.Goal.Validation),
				MaxTurns:     agent.Goal.MaxTurns,
				TokenBudget:  agent.Goal.TokenBudget,
				OnExhaustion: agent.Goal.OnExhaustion,
			}
		}
		out.Blocks = append(out.Blocks, block)
	}
	return out
}

func cloneStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}
