// Package model defines the in-memory representation of a flow.
package model

// Flow represents a complete agent orchestration flow parsed from a markdown file.
type Flow struct {
	Name           string
	Description    string
	ExternalInputs []string
	Defaults       Defaults
	Agents         []Agent
}

// Defaults holds flow-level default configuration inherited by all agents.
type Defaults struct {
	Model       string  `yaml:"model"`
	Temperature float64 `yaml:"temperature"`
}

// Agent represents a single agent node in the flow.
type Agent struct {
	Name     string
	Position [2]int
	Inputs   map[string]Input
	Start    []Condition
	NodeType NodeType
	Language string // empty for prompt nodes; "python", "bash", etc. for function nodes
	Content  string // the prompt text or code
	// Per-agent config overrides
	Model         string  `yaml:"model"`
	Temperature   float64 `yaml:"temperature"`
	OnError       string  `yaml:"on_error"`
	OnExhaustion  string  `yaml:"on_exhaustion"`
}

// Input represents a data source for an agent input.
type Input struct {
	From     string `yaml:"from"`
	Fallback string `yaml:"fallback"`
}

// Condition represents a start condition for an agent.
type Condition struct {
	Always       *AlwaysCondition `yaml:"always,omitempty"`
	When         StringOrList     `yaml:"when,omitempty"`
	Contains     string           `yaml:"contains,omitempty"`
	MaxRuns      int              `yaml:"max_runs,omitempty"`
	OnExhaustion string           `yaml:"on_exhaustion,omitempty"`
}

// AlwaysCondition represents an unconditional start with an optional run cap.
type AlwaysCondition struct {
	MaxRuns int `yaml:"max_runs"`
}

// StringOrList is a type that can be unmarshalled from either a single string
// or a list of strings in YAML.
type StringOrList []string

// NodeType distinguishes prompt nodes from function nodes.
type NodeType int

const (
	PromptNode   NodeType = iota // LLM prompt — content is markdown prose
	FunctionNode                 // Code block — content is executable code
)
