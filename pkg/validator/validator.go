// Package validator checks a parsed Flow for errors before execution.
package validator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/samleeney/flows/pkg/model"
)

// ValidationError represents a single validation failure.
type ValidationError struct {
	Agent   string // empty for flow-level errors
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Agent != "" {
		return fmt.Sprintf("agent %q field %q: %s", e.Agent, e.Field, e.Message)
	}
	return fmt.Sprintf("field %q: %s", e.Field, e.Message)
}

// ValidationErrors is a collection of validation failures.
type ValidationErrors []ValidationError

func (errs ValidationErrors) Error() string {
	var parts []string
	for _, e := range errs {
		parts = append(parts, e.Error())
	}
	return strings.Join(parts, "\n")
}

var identifierRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Validate checks a flow for structural errors. Returns nil if valid.
func Validate(flow *model.Flow) error {
	var errs ValidationErrors

	if flow.Name == "" {
		errs = append(errs, ValidationError{Field: "name", Message: "required"})
	}

	agentNames := make(map[string]bool)
	for _, a := range flow.Agents {
		agentNames[a.Name] = true
	}

	externalInputs := make(map[string]bool)
	for _, ei := range flow.ExternalInputs {
		externalInputs[ei] = true
	}

	exhaustionRouteTargets := findExhaustionRouteTargets(flow, agentNames)

	for _, agent := range flow.Agents {
		errs = append(errs, validateAgent(&agent, agentNames, externalInputs, exhaustionRouteTargets)...)
	}

	cycleErrs := validateCycles(flow)
	errs = append(errs, cycleErrs...)

	if len(errs) == 0 {
		return nil
	}
	return errs
}

func validateAgent(agent *model.Agent, agentNames, externalInputs, exhaustionRouteTargets map[string]bool) ValidationErrors {
	var errs ValidationErrors

	// Name must be a valid identifier
	if !identifierRe.MatchString(agent.Name) {
		errs = append(errs, ValidationError{
			Agent:   agent.Name,
			Field:   "name",
			Message: "must be lowercase alphanumeric with underscores, starting with a letter",
		})
	}

	// Validate inputs
	for name, input := range agent.Inputs {
		if input.From == "" {
			errs = append(errs, ValidationError{
				Agent:   agent.Name,
				Field:   fmt.Sprintf("inputs.%s", name),
				Message: "missing 'from' field",
			})
			continue
		}

		if input.From == "external" {
			// The input key must match a declared external input.
			if !externalInputs[name] {
				errs = append(errs, ValidationError{
					Agent:   agent.Name,
					Field:   fmt.Sprintf("inputs.%s", name),
					Message: fmt.Sprintf("references external input %q but it is not declared in external_inputs", name),
				})
			}
		} else if !agentNames[input.From] {
			errs = append(errs, ValidationError{
				Agent:   agent.Name,
				Field:   fmt.Sprintf("inputs.%s", name),
				Message: fmt.Sprintf("references unknown agent %q", input.From),
			})
		}

		if input.Fallback == "external" {
			if !externalInputs[name] {
				errs = append(errs, ValidationError{
					Agent:   agent.Name,
					Field:   fmt.Sprintf("inputs.%s.fallback", name),
					Message: fmt.Sprintf("references external input %q but it is not declared in external_inputs", name),
				})
			}
		} else if input.Fallback != "" && !agentNames[input.Fallback] {
			errs = append(errs, ValidationError{
				Agent:   agent.Name,
				Field:   fmt.Sprintf("inputs.%s.fallback", name),
				Message: fmt.Sprintf("references unknown agent %q", input.Fallback),
			})
		}
	}

	// Validate start conditions
	for i, cond := range agent.Start {
		for _, when := range cond.When {
			if !agentNames[when] {
				errs = append(errs, ValidationError{
					Agent:   agent.Name,
					Field:   fmt.Sprintf("start[%d].when", i),
					Message: fmt.Sprintf("references unknown agent %q", when),
				})
			}
		}
		if err := validateOnExhaustion(agent.Name, fmt.Sprintf("start[%d].on_exhaustion", i), cond.OnExhaustion, agentNames); err != nil {
			errs = append(errs, *err)
		}
	}

	if err := validateOnExhaustion(agent.Name, "on_exhaustion", agent.OnExhaustion, agentNames); err != nil {
		errs = append(errs, *err)
	}

	if agent.Goal != nil && strings.TrimSpace(agent.Goal.Objective) == "" {
		errs = append(errs, ValidationError{
			Agent:   agent.Name,
			Field:   "goal.objective",
			Message: "required",
		})
	}

	// Must have at least one start condition unless this agent is only reached
	// through an exhaustion route.
	if len(agent.Start) == 0 && !exhaustionRouteTargets[agent.Name] {
		errs = append(errs, ValidationError{
			Agent:   agent.Name,
			Field:   "start",
			Message: "at least one start condition required",
		})
	}

	// Must have content
	if agent.Content == "" {
		errs = append(errs, ValidationError{
			Agent:   agent.Name,
			Field:   "content",
			Message: "agent has no prompt or code content",
		})
	}

	return errs
}

func findExhaustionRouteTargets(flow *model.Flow, agentNames map[string]bool) map[string]bool {
	targets := make(map[string]bool)
	for _, agent := range flow.Agents {
		if isExhaustionRoute(agent.OnExhaustion, agentNames) {
			targets[agent.OnExhaustion] = true
		}
		for _, cond := range agent.Start {
			if isExhaustionRoute(cond.OnExhaustion, agentNames) {
				targets[cond.OnExhaustion] = true
			}
		}
	}
	return targets
}

func validateOnExhaustion(agentName, field, value string, agentNames map[string]bool) *ValidationError {
	value = strings.TrimSpace(value)
	if value == "" || isExhaustionPolicy(value) {
		return nil
	}
	if value == agentName {
		return &ValidationError{
			Agent:   agentName,
			Field:   field,
			Message: "cannot route to the same agent",
		}
	}
	if !agentNames[value] {
		return &ValidationError{
			Agent:   agentName,
			Field:   field,
			Message: fmt.Sprintf("unsupported policy or unknown route target %q", value),
		}
	}
	return nil
}

func isExhaustionRoute(value string, agentNames map[string]bool) bool {
	value = strings.TrimSpace(value)
	return value != "" && !isExhaustionPolicy(value) && agentNames[value]
}

func isExhaustionPolicy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "stop", "continue":
		return true
	default:
		return false
	}
}

// validateCycles detects cycles in the agent dependency graph and ensures
// every cycle has at least one max_runs cap.
func validateCycles(flow *model.Flow) ValidationErrors {
	var errs ValidationErrors

	// Build adjacency list from start conditions and inputs.
	// An edge from A to B means "B depends on A" (A must run before B).
	// We look for cycles in this graph.
	adj := make(map[string][]string)
	agentMap := make(map[string]*model.Agent)

	for i := range flow.Agents {
		a := &flow.Agents[i]
		agentMap[a.Name] = a
	}

	for _, agent := range flow.Agents {
		deps := agentDependencies(&agent)
		for _, dep := range deps {
			if _, ok := agentMap[dep]; ok {
				adj[dep] = append(adj[dep], agent.Name)
			}
		}
		for _, target := range exhaustionRoutes(&agent) {
			if _, ok := agentMap[target]; ok {
				adj[agent.Name] = append(adj[agent.Name], target)
			}
		}
	}

	// Find all cycles using DFS
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int)
	parent := make(map[string]string)

	var dfs func(node string) [][]string
	dfs = func(node string) [][]string {
		var cycles [][]string
		color[node] = gray
		for _, next := range adj[node] {
			if color[next] == gray {
				// Found a cycle — trace it back
				cycle := []string{next}
				cur := node
				for cur != next {
					cycle = append([]string{cur}, cycle...)
					cur = parent[cur]
				}
				cycle = append([]string{next}, cycle...)
				cycles = append(cycles, cycle)
			} else if color[next] == white {
				parent[next] = node
				cycles = append(cycles, dfs(next)...)
			}
		}
		color[node] = black
		return cycles
	}

	for _, agent := range flow.Agents {
		if color[agent.Name] == white {
			cycles := dfs(agent.Name)
			for _, cycle := range cycles {
				if !cycleHasMaxRuns(cycle, agentMap) {
					errs = append(errs, ValidationError{
						Field:   "cycle",
						Message: fmt.Sprintf("cycle %v has no max_runs cap, would loop forever", cycle),
					})
				}
			}
		}
	}

	return errs
}

// agentDependencies returns the names of agents this agent depends on
// (from start conditions and inputs).
func agentDependencies(agent *model.Agent) []string {
	seen := make(map[string]bool)
	var deps []string

	for _, cond := range agent.Start {
		for _, w := range cond.When {
			if !seen[w] {
				seen[w] = true
				deps = append(deps, w)
			}
		}
	}

	for _, input := range agent.Inputs {
		if input.From != "external" && input.From != "" && !seen[input.From] {
			seen[input.From] = true
			deps = append(deps, input.From)
		}
	}

	return deps
}

func exhaustionRoutes(agent *model.Agent) []string {
	seen := make(map[string]bool)
	var routes []string
	if agent.OnExhaustion != "" && !isExhaustionPolicy(agent.OnExhaustion) {
		seen[agent.OnExhaustion] = true
		routes = append(routes, agent.OnExhaustion)
	}
	for _, cond := range agent.Start {
		if cond.OnExhaustion != "" && !isExhaustionPolicy(cond.OnExhaustion) && !seen[cond.OnExhaustion] {
			seen[cond.OnExhaustion] = true
			routes = append(routes, cond.OnExhaustion)
		}
	}
	return routes
}

// cycleHasMaxRuns checks that at least one agent in the cycle has a max_runs cap.
func cycleHasMaxRuns(cycle []string, agentMap map[string]*model.Agent) bool {
	seen := make(map[string]bool)
	for _, name := range cycle {
		if seen[name] {
			continue
		}
		seen[name] = true
		agent, ok := agentMap[name]
		if !ok {
			continue
		}
		for _, cond := range agent.Start {
			if cond.MaxRuns > 0 {
				return true
			}
			if cond.Always != nil && cond.Always.MaxRuns > 0 {
				return true
			}
		}
	}
	return false
}
