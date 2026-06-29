package parser

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/samleeney/flows/pkg/model"
)

func TestParseCodeReviewFlow(t *testing.T) {
	flow, err := ParseFile(filepath.Join("testdata", "code_review.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Flow-level fields
	if flow.Name != "Code Review Flow" {
		t.Errorf("name = %q, want %q", flow.Name, "Code Review Flow")
	}
	if flow.Description != "Automated review-fix loop with merge" {
		t.Errorf("description = %q, want %q", flow.Description, "Automated review-fix loop with merge")
	}
	if len(flow.ExternalInputs) != 2 {
		t.Fatalf("external_inputs len = %d, want 2", len(flow.ExternalInputs))
	}
	if flow.ExternalInputs[0] != "code" || flow.ExternalInputs[1] != "guidelines" {
		t.Errorf("external_inputs = %v, want [code, guidelines]", flow.ExternalInputs)
	}
	if flow.Defaults.Model != "claude-sonnet-4-20250514" {
		t.Errorf("defaults.model = %q, want %q", flow.Defaults.Model, "claude-sonnet-4-20250514")
	}
	if flow.Defaults.Temperature != 0.3 {
		t.Errorf("defaults.temperature = %f, want 0.3", flow.Defaults.Temperature)
	}

	// Agents
	if len(flow.Agents) != 3 {
		t.Fatalf("agents len = %d, want 3", len(flow.Agents))
	}

	// Reviewer
	reviewer := flow.Agents[0]
	if reviewer.Name != "reviewer" {
		t.Errorf("agent[0].name = %q, want %q", reviewer.Name, "reviewer")
	}
	if reviewer.Position != [2]int{0, 0} {
		t.Errorf("reviewer.position = %v, want [0,0]", reviewer.Position)
	}
	if reviewer.NodeType != model.PromptNode {
		t.Errorf("reviewer.nodeType = %v, want PromptNode", reviewer.NodeType)
	}
	if len(reviewer.Inputs) != 2 {
		t.Errorf("reviewer.inputs len = %d, want 2", len(reviewer.Inputs))
	}
	codeInput := reviewer.Inputs["code"]
	if codeInput.From != "fixer" || codeInput.Fallback != "external" {
		t.Errorf("reviewer.inputs[code] = %+v, want from=fixer fallback=external", codeInput)
	}
	guidelinesInput := reviewer.Inputs["guidelines"]
	if guidelinesInput.From != "external" {
		t.Errorf("reviewer.inputs[guidelines] = %+v, want from=external", guidelinesInput)
	}
	if len(reviewer.Start) != 2 {
		t.Fatalf("reviewer.start len = %d, want 2", len(reviewer.Start))
	}
	if reviewer.Start[0].Always == nil || reviewer.Start[0].Always.MaxRuns != 1 {
		t.Errorf("reviewer.start[0] = %+v, want always with max_runs=1", reviewer.Start[0])
	}
	if len(reviewer.Start[1].When) != 1 || reviewer.Start[1].When[0] != "fixer" {
		t.Errorf("reviewer.start[1].when = %v, want [fixer]", reviewer.Start[1].When)
	}
	if reviewer.Start[1].MaxRuns != 5 {
		t.Errorf("reviewer.start[1].max_runs = %d, want 5", reviewer.Start[1].MaxRuns)
	}
	if reviewer.Content == "" {
		t.Error("reviewer.content is empty")
	}
	// Verify no text duplication in content (regression for parser double-counting)
	phrase := "Review the provided code against the guidelines."
	if n := strings.Count(reviewer.Content, phrase); n != 1 {
		t.Errorf("reviewer.content contains %q %d times, want exactly 1", phrase, n)
	}

	// Fixer
	fixer := flow.Agents[1]
	if fixer.Name != "fixer" {
		t.Errorf("agent[1].name = %q, want %q", fixer.Name, "fixer")
	}
	if fixer.Position != [2]int{1, 1} {
		t.Errorf("fixer.position = %v, want [1,1]", fixer.Position)
	}
	if len(fixer.Start) != 1 {
		t.Fatalf("fixer.start len = %d, want 1", len(fixer.Start))
	}
	if fixer.Start[0].Contains != "needs_changes" {
		t.Errorf("fixer.start[0].contains = %q, want %q", fixer.Start[0].Contains, "needs_changes")
	}

	// Merger
	merger := flow.Agents[2]
	if merger.Name != "merger" {
		t.Errorf("agent[2].name = %q, want %q", merger.Name, "merger")
	}
	if merger.Start[0].Contains != "approved" {
		t.Errorf("merger.start[0].contains = %q, want %q", merger.Start[0].Contains, "approved")
	}
	mergerCode := merger.Inputs["code"]
	if mergerCode.From != "fixer" || mergerCode.Fallback != "external" {
		t.Errorf("merger.inputs[code] = %+v, want from=fixer fallback=external", mergerCode)
	}
}

func TestParseFunctionNode(t *testing.T) {
	flow, err := ParseFile(filepath.Join("testdata", "function_node.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Agents) != 2 {
		t.Fatalf("agents len = %d, want 2", len(flow.Agents))
	}

	processor := flow.Agents[0]
	if processor.NodeType != model.FunctionNode {
		t.Errorf("processor.nodeType = %v, want FunctionNode", processor.NodeType)
	}
	if processor.Language != "python" {
		t.Errorf("processor.language = %q, want %q", processor.Language, "python")
	}
	if processor.Content == "" {
		t.Error("processor.content is empty")
	}

	reporter := flow.Agents[1]
	if reporter.NodeType != model.PromptNode {
		t.Errorf("reporter.nodeType = %v, want PromptNode", reporter.NodeType)
	}
}

func TestParseGoalBlock(t *testing.T) {
	src := []byte(`---
name: Goal Flow
external_inputs:
  - topic
---

## writer

` + "```yaml" + `
inputs:
  topic: { from: external }
start:
  - always: { max_runs: 1 }
` + "```" + `

` + "```goal" + `
objective: Write exactly three concise bullets.
validation:
  - Exactly three lines.
  - Each line starts with "- ".
max_turns: 2
on_exhaustion: escalate
` + "```" + `

Write the bullets for the supplied topic.
`)

	flow, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(flow.Agents) != 1 {
		t.Fatalf("agents len = %d, want 1", len(flow.Agents))
	}
	agent := flow.Agents[0]
	if agent.Goal == nil {
		t.Fatal("goal was not parsed")
	}
	if agent.Goal.Objective != "Write exactly three concise bullets." {
		t.Fatalf("goal objective = %q", agent.Goal.Objective)
	}
	if len(agent.Goal.Validation) != 2 {
		t.Fatalf("validation len = %d, want 2", len(agent.Goal.Validation))
	}
	if agent.Goal.MaxTurns != 2 {
		t.Fatalf("max_turns = %d, want 2", agent.Goal.MaxTurns)
	}
	if agent.Goal.OnExhaustion != "escalate" {
		t.Fatalf("on_exhaustion = %q, want escalate", agent.Goal.OnExhaustion)
	}
	if agent.NodeType != model.PromptNode {
		t.Fatalf("node type = %v, want prompt", agent.NodeType)
	}
	if strings.Contains(agent.Content, "```goal") {
		t.Fatalf("goal block leaked into content: %q", agent.Content)
	}
	if !strings.Contains(agent.Content, "Write the bullets") {
		t.Fatalf("prompt content missing: %q", agent.Content)
	}
}

func TestParseMissingFrontmatter(t *testing.T) {
	src := []byte("## agent\n\nSome content\n")
	_, err := Parse(src)
	if err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestParseInvalidFrontmatter(t *testing.T) {
	src := []byte("---\ndescription: no name\n---\n\n## agent\n\nContent\n")
	_, err := Parse(src)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestExtractFrontmatter(t *testing.T) {
	src := []byte("---\nname: Test\n---\n\n## agent\n\nContent\n")
	body, fm, err := extractFrontmatter(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(fm) != "\nname: Test" {
		t.Errorf("frontmatter = %q, want %q", string(fm), "\nname: Test")
	}
	if len(body) == 0 {
		t.Error("body is empty")
	}
}

func TestExtractFrontmatterUnterminated(t *testing.T) {
	src := []byte("---\nname: Test\nno closing\n")
	_, _, err := extractFrontmatter(src)
	if err == nil {
		t.Fatal("expected error for unterminated frontmatter")
	}
}
