// Package runtime executes a validated Flow using a reactive dataflow model.
package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/samleeney/flows/pkg/model"
)

// RunOptions configures a flow execution.
type RunOptions struct {
	ExternalInputs map[string]string
	Verbose        bool
	OnAgentStart   func(name string, iteration int)
	OnAgentDone    func(name string, iteration int, output string, err error)
}

// RunResult contains the outputs of a completed flow execution.
type RunResult struct {
	Outputs map[string]string // agent name → last output
}

// Run executes a flow to completion.
func Run(ctx context.Context, flow *model.Flow, registry *ExecutorRegistry, opts RunOptions) (*RunResult, error) {
	if opts.ExternalInputs == nil {
		opts.ExternalInputs = make(map[string]string)
	}

	state := &flowState{
		flow:     flow,
		registry: registry,
		opts:     opts,
		outputs:  make(map[string]string),
		runs:     make(map[string]int),
		agents:   make(map[string]*model.Agent),
	}

	for i := range flow.Agents {
		state.agents[flow.Agents[i].Name] = &flow.Agents[i]
	}

	return state.run(ctx)
}

type flowState struct {
	flow     *model.Flow
	registry *ExecutorRegistry
	opts     RunOptions
	mu       sync.Mutex
	outputs  map[string]string // agent name → latest output
	runs     map[string]int    // agent name → invocation count
	agents   map[string]*model.Agent
}

func (s *flowState) run(ctx context.Context) (*RunResult, error) {
	for {
		ready := s.findReady()
		if len(ready) == 0 {
			break
		}

		// Execute all ready agents in parallel
		var wg sync.WaitGroup
		errCh := make(chan error, len(ready))

		for _, agent := range ready {
			wg.Add(1)
			go func(a *model.Agent) {
				defer wg.Done()
				if err := s.executeAgent(ctx, a); err != nil {
					errCh <- fmt.Errorf("agent %q: %w", a.Name, err)
				}
			}(agent)
		}

		wg.Wait()
		close(errCh)

		// Collect errors
		for err := range errCh {
			return nil, err
		}
	}

	result := &RunResult{Outputs: make(map[string]string)}
	s.mu.Lock()
	for k, v := range s.outputs {
		result.Outputs[k] = v
	}
	s.mu.Unlock()

	return result, nil
}

// findReady returns agents whose start conditions are met and inputs are
// available. Thread-safe.
func (s *flowState) findReady() []*model.Agent {
	s.mu.Lock()
	defer s.mu.Unlock()

	var ready []*model.Agent
	for i := range s.flow.Agents {
		agent := &s.flow.Agents[i]
		if s.canFire(agent) {
			ready = append(ready, agent)
		}
	}
	return ready
}

func (s *flowState) canFire(agent *model.Agent) bool {
	// Check if any start condition is met
	condMet := false
	for _, cond := range agent.Start {
		if s.conditionMet(agent.Name, &cond) {
			condMet = true
			break
		}
	}
	if !condMet {
		return false
	}

	// Check all inputs are available
	for _, input := range agent.Inputs {
		if !s.inputAvailable(&input) {
			return false
		}
	}

	return true
}

func (s *flowState) conditionMet(agentName string, cond *model.Condition) bool {
	runs := s.runs[agentName]

	// Check global max_runs for this condition
	if cond.MaxRuns > 0 && runs >= cond.MaxRuns {
		return false
	}

	if cond.Always != nil {
		if cond.Always.MaxRuns > 0 && runs >= cond.Always.MaxRuns {
			return false
		}
		return true
	}

	if len(cond.When) > 0 {
		// All referenced agents must have produced output
		for _, dep := range cond.When {
			output, hasOutput := s.outputs[dep]
			if !hasOutput {
				return false
			}
			// Check contains constraint
			if cond.Contains != "" && !strings.Contains(output, cond.Contains) {
				return false
			}
		}

		// The "when" condition also requires new output since last run.
		// We track this by checking if the dependency has run more recently.
		// For simplicity: condition met if dependency has output and we haven't
		// already consumed it at this run count.
		if runs > 0 {
			// On subsequent runs, we need the dependency to have produced
			// NEW output (i.e., the dependency ran after our last run).
			// We approximate this: if we already ran N times and dependency
			// output hasn't changed, don't re-fire.
			// This is tracked by checking if dependency run count > our run count - 1
			for _, dep := range cond.When {
				depRuns := s.runs[dep]
				if depRuns <= runs-1 {
					return false
				}
			}
		}

		return true
	}

	return false
}

func (s *flowState) inputAvailable(input *model.Input) bool {
	if input.From == "external" {
		_, ok := s.opts.ExternalInputs[input.From]
		// External inputs are always "available" — they're provided at start
		return true || ok
	}

	_, hasOutput := s.outputs[input.From]
	if hasOutput {
		return true
	}

	// Check fallback
	if input.Fallback == "external" {
		return true
	}
	if input.Fallback != "" {
		_, hasFallback := s.outputs[input.Fallback]
		return hasFallback
	}

	return false
}

func (s *flowState) resolveInputs(agent *model.Agent) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	resolved := make(map[string]string)
	for name, input := range agent.Inputs {
		if input.From == "external" {
			resolved[name] = s.opts.ExternalInputs[name]
			continue
		}

		if output, ok := s.outputs[input.From]; ok {
			resolved[name] = output
			continue
		}

		// Use fallback
		if input.Fallback == "external" {
			resolved[name] = s.opts.ExternalInputs[name]
		} else if input.Fallback != "" {
			resolved[name] = s.outputs[input.Fallback]
		}
	}
	return resolved
}

func (s *flowState) executeAgent(ctx context.Context, agent *model.Agent) error {
	s.mu.Lock()
	iteration := s.runs[agent.Name] + 1
	s.runs[agent.Name] = iteration
	s.mu.Unlock()

	if s.opts.OnAgentStart != nil {
		s.opts.OnAgentStart(agent.Name, iteration)
	}

	inputs := s.resolveInputs(agent)

	lang := agent.Language
	if agent.NodeType == model.PromptNode {
		lang = ""
	}

	executor, err := s.registry.Get(lang)
	if err != nil {
		return err
	}

	output, err := executor.Execute(ctx, agent.Content, inputs)
	if s.opts.OnAgentDone != nil {
		s.opts.OnAgentDone(agent.Name, iteration, output, err)
	}
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.outputs[agent.Name] = output
	s.mu.Unlock()

	return nil
}
