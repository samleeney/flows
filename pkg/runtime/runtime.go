// Package runtime executes a validated Flow using a reactive dataflow model.
package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/samleeney/flows/pkg/live"
	"github.com/samleeney/flows/pkg/model"
)

// PreviewMaxBytes caps the size of an output preview attached to a live event.
const PreviewMaxBytes = 4096

// RunOptions configures a flow execution.
type RunOptions struct {
	ExternalInputs map[string]string
	Verbose        bool
	OnAgentStart   func(name string, iteration int)
	OnAgentDone    func(name string, iteration int, output string, err error)

	// Live event emission. Both fields are optional; if Observer is nil, a
	// NopObserver is used and FlowKey is ignored.
	FlowKey  string
	Observer live.Observer
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
	if opts.Observer == nil {
		opts.Observer = live.NopObserver{}
	}

	runID, err := live.NewRunID()
	if err != nil {
		return nil, fmt.Errorf("generate run id: %w", err)
	}

	state := &flowState{
		flow:     flow,
		registry: registry,
		opts:     opts,
		runID:    runID,
		outputs:  make(map[string]string),
		runs:     make(map[string]int),
		consumed: make(map[string]map[string]int),
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
	runID    string

	mu       sync.Mutex
	outputs  map[string]string         // agent name → latest output
	runs     map[string]int            // agent name → invocation count
	consumed map[string]map[string]int // consumer → {producer → producer.runs at last consumption}
	agents   map[string]*model.Agent

	// Live event emission state. emitMu serializes seq allocation AND
	// observer publish so ordering at the queue is strict.
	emitMu sync.Mutex
	seq    uint64
}

func (s *flowState) run(ctx context.Context) (result *RunResult, err error) {
	s.emitRunStarted()
	defer func() {
		ok := err == nil
		var errStr string
		if err != nil {
			errStr = err.Error()
		}
		s.emitRunFinished(ok, errStr)
	}()

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

		for e := range errCh {
			err = e
			return nil, err
		}
	}

	result = &RunResult{Outputs: make(map[string]string)}
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

	for _, input := range agent.Inputs {
		if !s.inputAvailable(&input) {
			return false
		}
	}

	return true
}

func (s *flowState) conditionMet(agentName string, cond *model.Condition) bool {
	runs := s.runs[agentName]

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
		for _, dep := range cond.When {
			output, hasOutput := s.outputs[dep]
			if !hasOutput {
				return false
			}
			if cond.Contains != "" && !strings.Contains(output, cond.Contains) {
				return false
			}
			depRuns := s.runs[dep]
			consumed := s.consumed[agentName][dep]
			if depRuns <= consumed {
				return false
			}
		}
		return true
	}

	return false
}

func (s *flowState) inputAvailable(input *model.Input) bool {
	if input.From == "external" {
		_, ok := s.opts.ExternalInputs[input.From]
		return true || ok
	}

	_, hasOutput := s.outputs[input.From]
	if hasOutput {
		return true
	}

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
	if s.consumed[agent.Name] == nil {
		s.consumed[agent.Name] = make(map[string]int)
	}
	for _, cond := range agent.Start {
		for _, dep := range cond.When {
			s.consumed[agent.Name][dep] = s.runs[dep]
		}
	}
	for _, input := range agent.Inputs {
		if input.From != "external" && input.From != "" {
			s.consumed[agent.Name][input.From] = s.runs[input.From]
		}
	}
	s.mu.Unlock()

	if s.opts.OnAgentStart != nil {
		s.opts.OnAgentStart(agent.Name, iteration)
	}
	s.emitAgentStarted(agent.Name, iteration)

	inputs := s.resolveInputs(agent)

	lang := agent.Language
	if agent.NodeType == model.PromptNode {
		lang = ""
	}

	executor, err := s.registry.Get(lang)
	if err != nil {
		s.emitAgentFinished(agent.Name, iteration, live.StatusFailed, 0, "", err)
		return err
	}

	startTime := time.Now()
	output, err := executeNode(ctx, executor, ExecutionRequest{
		FlowName: s.flow.Name,
		Defaults: s.flow.Defaults,
		Agent:    *agent,
		Content:  agent.Content,
		Inputs:   inputs,
	})
	durationMS := time.Since(startTime).Milliseconds()

	if s.opts.OnAgentDone != nil {
		s.opts.OnAgentDone(agent.Name, iteration, output, err)
	}

	if err != nil {
		s.emitAgentFinished(agent.Name, iteration, live.StatusFailed, durationMS, "", err)
		return err
	}

	s.emitAgentFinished(agent.Name, iteration, live.StatusDone, durationMS, output, nil)

	s.mu.Lock()
	s.outputs[agent.Name] = output
	s.mu.Unlock()

	return nil
}

func executeNode(ctx context.Context, executor Executor, req ExecutionRequest) (string, error) {
	if ae, ok := executor.(AgentExecutor); ok {
		return ae.ExecuteAgent(ctx, req)
	}
	return executor.Execute(ctx, req.Content, req.Inputs)
}

// emit constructs and publishes a single envelope. Holds emitMu for both seq
// allocation and the (non-blocking) Publish, so ordering at the observer
// queue is strict per run.
func (s *flowState) emit(env live.EventEnvelope) {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.seq++
	env.Version = live.ProtocolVersion
	env.FlowKey = s.opts.FlowKey
	env.RunID = s.runID
	env.Seq = s.seq
	env.TS = time.Now().UTC()
	_ = s.opts.Observer.Publish(env)
}

func (s *flowState) emitRunStarted() {
	s.emit(live.EventEnvelope{Kind: live.KindRunStarted})
}

func (s *flowState) emitAgentStarted(name string, iter int) {
	s.emit(live.EventEnvelope{
		Kind:  live.KindAgentStarted,
		Agent: name,
		Iter:  iter,
	})
}

func (s *flowState) emitAgentFinished(name string, iter int, status live.AgentStatus, durationMS int64, output string, execErr error) {
	env := live.EventEnvelope{
		Kind:       live.KindAgentFinished,
		Agent:      name,
		Iter:       iter,
		Status:     status,
		DurationMS: durationMS,
	}
	if status == live.StatusDone && output != "" {
		preview, total, truncated := live.TruncatePreviewUTF8(output, PreviewMaxBytes)
		env.OutputPreview = preview
		env.OutputBytes = total
		env.OutputTruncated = truncated
	}
	if execErr != nil {
		env.Error = execErr.Error()
	}
	s.emit(env)
}

func (s *flowState) emitRunFinished(ok bool, errStr string) {
	v := ok
	s.emit(live.EventEnvelope{
		Kind:  live.KindRunFinished,
		OK:    &v,
		Error: errStr,
	})
}
