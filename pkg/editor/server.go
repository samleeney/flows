// Package editor provides the HTTP/WebSocket server for the visual flow editor.
package editor

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/samleeney/flows/pkg/live"
	"github.com/samleeney/flows/pkg/model"
	"github.com/samleeney/flows/pkg/parser"
	"github.com/samleeney/flows/pkg/serializer"
)

// NewServerOptions configures a new editor Server.
type NewServerOptions struct {
	FilePath      string          // path to the flow .md file
	CanonicalPath string          // optional canonical path for live discovery
	FlowKey       string          // optional sha256 of CanonicalPath
	Token         string          // empty → live endpoints not registered
	UIFS          http.FileSystem // nil → fallback placeholder index
}

// Server is the editor backend.
type Server struct {
	opts NewServerOptions

	mu       sync.RWMutex
	flow     *model.Flow
	clientMu sync.Mutex
	clients  map[*wsClient]struct{}
	upgrader websocket.Upgrader
	watcher  *FileWatcher

	runStore *RunStore

	httpServer *http.Server
}

// NewServer creates a new editor server.
func NewServer(opts NewServerOptions) (*Server, error) {
	if opts.FilePath == "" {
		return nil, errors.New("editor: FilePath is required")
	}
	flow, err := parser.ParseFile(opts.FilePath)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", opts.FilePath, err)
	}
	s := &Server{
		opts:    opts,
		flow:    flow,
		clients: make(map[*wsClient]struct{}),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		runStore: NewRunStore(opts.FlowKey),
	}
	return s, nil
}

// FlowKey returns the configured flow_key.
func (s *Server) FlowKey() string { return s.opts.FlowKey }

// CanonicalPath returns the configured canonical flow path.
func (s *Server) CanonicalPath() string { return s.opts.CanonicalPath }

// RunSnapshot returns the current state of the live run store.
func (s *Server) RunSnapshot() RunSnapshot { return s.runStore.Snapshot() }

// Serve binds the editor to the given listener and serves until Close is called.
func (s *Server) Serve(ln net.Listener) error {
	s.httpServer = &http.Server{Handler: s.Handler()}
	err := s.httpServer.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// FlowJSON is the JSON representation sent to the frontend.
type FlowJSON struct {
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	ExternalInputs []string     `json:"external_inputs"`
	Defaults       DefaultsJSON `json:"defaults"`
	Agents         []AgentJSON  `json:"agents"`
}

type DefaultsJSON struct {
	PromptExecutor string  `json:"prompt_executor,omitempty"`
	Model          string  `json:"model,omitempty"`
	Temperature    float64 `json:"temperature,omitempty"`
}

type AgentJSON struct {
	Name           string               `json:"name"`
	Position       [2]int               `json:"position"`
	Inputs         map[string]InputJSON `json:"inputs"`
	Start          []ConditionJSON      `json:"start"`
	Goal           *GoalJSON            `json:"goal,omitempty"`
	NodeType       string               `json:"node_type"`
	Language       string               `json:"language,omitempty"`
	Content        string               `json:"content"`
	PromptExecutor string               `json:"prompt_executor,omitempty"`
	Model          string               `json:"model,omitempty"`
	Temperature    float64              `json:"temperature,omitempty"`
	OnError        string               `json:"on_error,omitempty"`
	OnExhaustion   string               `json:"on_exhaustion,omitempty"`
}

type GoalJSON struct {
	Objective    string   `json:"objective"`
	Validation   []string `json:"validation,omitempty"`
	MaxTurns     int      `json:"max_turns,omitempty"`
	TokenBudget  int      `json:"token_budget,omitempty"`
	OnExhaustion string   `json:"on_exhaustion,omitempty"`
}

type InputJSON struct {
	From     string `json:"from"`
	Fallback string `json:"fallback,omitempty"`
}

type ConditionJSON struct {
	Always       *AlwaysJSON `json:"always,omitempty"`
	When         []string    `json:"when,omitempty"`
	Contains     string      `json:"contains,omitempty"`
	MaxRuns      int         `json:"max_runs,omitempty"`
	OnExhaustion string      `json:"on_exhaustion,omitempty"`
}

type AlwaysJSON struct {
	MaxRuns int `json:"max_runs"`
}

// WSMessage is a WebSocket message envelope.
type WSMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Handler returns the http.Handler for the editor.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/flow", s.handleFlow)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/api/live/runs", s.handleLiveRuns)
	if s.opts.Token != "" {
		mux.HandleFunc("/api/live/health", s.handleLiveHealth)
		mux.HandleFunc("/api/live/events", s.handleLiveEvents)
	}
	if s.opts.UIFS != nil {
		mux.Handle("/", http.FileServer(s.opts.UIFS))
	} else {
		mux.HandleFunc("/", s.handleIndex)
	}
	return mux
}

func (s *Server) handleFlow(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		flowJSON := flowToJSON(s.flow)
		s.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(flowJSON)

	case http.MethodPut:
		var fj FlowJSON
		if err := json.NewDecoder(r.Body).Decode(&fj); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		flow := jsonToFlow(&fj)
		s.mu.Lock()
		s.flow = flow
		s.mu.Unlock()
		if err := s.writeFile(flow); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.broadcastFlowExcept(flow, nil)
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleLiveRuns(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.runStore.Snapshot())
}

func (s *Server) handleLiveHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"version":  live.ProtocolVersion,
		"flow_key": s.opts.FlowKey,
		"pid":      os.Getpid(),
	})
}

func (s *Server) handleLiveEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth := r.Header.Get("Authorization")
	expected := "Bearer " + s.opts.Token
	if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	dec := json.NewDecoder(r.Body)
	var lastRunID string
	for {
		var env live.EventEnvelope
		if err := dec.Decode(&env); err != nil {
			break
		}
		if env.FlowKey != s.opts.FlowKey {
			http.Error(w, "flow key mismatch", http.StatusConflict)
			return
		}
		if s.runStore.Apply(env) {
			s.broadcastRunEvent(env)
		}
		lastRunID = env.RunID
		if env.Kind == live.KindRunFinished {
			lastRunID = ""
		}
	}
	if lastRunID != "" {
		s.runStore.MarkDisconnected(lastRunID)
		s.broadcastRunSnapshot()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade: %v", err)
		return
	}
	client := newWSClient(conn)

	s.clientMu.Lock()
	s.clients[client] = struct{}{}
	s.clientMu.Unlock()

	// Initial state: flow, then run_snapshot.
	s.mu.RLock()
	flowJSON := flowToJSON(s.flow)
	s.mu.RUnlock()
	if data, err := json.Marshal(WSMessage{Type: "flow", Data: mustJSON(flowJSON)}); err == nil {
		client.Send(data)
	}
	snapshot := s.runStore.Snapshot()
	if data, err := json.Marshal(WSMessage{Type: "run_snapshot", Data: mustJSON(snapshot)}); err == nil {
		client.Send(data)
	}

	// Read loop runs until conn closes.
	for {
		var wsMsg WSMessage
		if err := conn.ReadJSON(&wsMsg); err != nil {
			break
		}
		switch wsMsg.Type {
		case "update_flow":
			var fj FlowJSON
			if err := json.Unmarshal(wsMsg.Data, &fj); err != nil {
				log.Printf("invalid flow update: %v", err)
				continue
			}
			flow := jsonToFlow(&fj)
			s.mu.Lock()
			s.flow = flow
			s.mu.Unlock()
			if err := s.writeFile(flow); err != nil {
				log.Printf("write file: %v", err)
			}
			s.broadcastFlowExcept(flow, client)
		}
	}

	s.clientMu.Lock()
	delete(s.clients, client)
	s.clientMu.Unlock()
	client.Close()
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Flow Editor</title></head>
<body>
<h1>Flow Editor</h1>
<p>React Flow frontend will be embedded here.</p>
<div id="root"></div>
</body>
</html>`))
}

func (s *Server) writeFile(flow *model.Flow) error {
	data, err := serializer.Serialize(flow)
	if err != nil {
		return err
	}
	return os.WriteFile(s.opts.FilePath, data, 0o644)
}

func (s *Server) broadcastFlowExcept(flow *model.Flow, exclude *wsClient) {
	flowJSON := flowToJSON(flow)
	data, err := json.Marshal(WSMessage{Type: "flow", Data: mustJSON(flowJSON)})
	if err != nil {
		return
	}
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	for c := range s.clients {
		if c == exclude {
			continue
		}
		c.Send(data)
	}
}

func (s *Server) broadcastRunEvent(env live.EventEnvelope) {
	data, err := json.Marshal(WSMessage{Type: "run_event", Data: mustJSON(env)})
	if err != nil {
		return
	}
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	for c := range s.clients {
		c.Send(data)
	}
}

func (s *Server) broadcastRunSnapshot() {
	snap := s.runStore.Snapshot()
	data, err := json.Marshal(WSMessage{Type: "run_snapshot", Data: mustJSON(snap)})
	if err != nil {
		return
	}
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	for c := range s.clients {
		c.Send(data)
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

// StartFileWatcher begins watching the flow file for external changes.
func (s *Server) StartFileWatcher() error {
	watcher, err := NewFileWatcher(s.opts.FilePath, func() {
		flow, err := parser.ParseFile(s.opts.FilePath)
		if err != nil {
			log.Printf("reparse on file change: %v", err)
			return
		}
		s.mu.Lock()
		s.flow = flow
		s.mu.Unlock()
		s.broadcastFlowExcept(flow, nil)
	})
	if err != nil {
		return err
	}
	s.watcher = watcher
	return nil
}

// Close shuts down the server, file watcher, and all WebSocket clients. It is
// idempotent and bounds graceful shutdown to ~200ms.
func (s *Server) Close() {
	if s.watcher != nil {
		s.watcher.Close()
		s.watcher = nil
	}
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		_ = s.httpServer.Shutdown(ctx)
		cancel()
		s.httpServer = nil
	}
	s.clientMu.Lock()
	for c := range s.clients {
		c.Close()
		delete(s.clients, c)
	}
	s.clientMu.Unlock()
}

// Conversion functions between model and JSON types.

func flowToJSON(flow *model.Flow) FlowJSON {
	agents := make([]AgentJSON, len(flow.Agents))
	for i, a := range flow.Agents {
		agents[i] = agentToJSON(&a)
	}
	return FlowJSON{
		Name:           flow.Name,
		Description:    flow.Description,
		ExternalInputs: flow.ExternalInputs,
		Defaults: DefaultsJSON{
			PromptExecutor: flow.Defaults.PromptExecutor,
			Model:          flow.Defaults.Model,
			Temperature:    flow.Defaults.Temperature,
		},
		Agents: agents,
	}
}

func agentToJSON(a *model.Agent) AgentJSON {
	inputs := make(map[string]InputJSON)
	for k, v := range a.Inputs {
		inputs[k] = InputJSON{From: v.From, Fallback: v.Fallback}
	}

	conditions := make([]ConditionJSON, len(a.Start))
	for i, c := range a.Start {
		cj := ConditionJSON{
			When:         []string(c.When),
			Contains:     c.Contains,
			MaxRuns:      c.MaxRuns,
			OnExhaustion: c.OnExhaustion,
		}
		if c.Always != nil {
			cj.Always = &AlwaysJSON{MaxRuns: c.Always.MaxRuns}
		}
		conditions[i] = cj
	}

	nodeType := "prompt"
	if a.NodeType == model.FunctionNode {
		nodeType = "function"
	}

	return AgentJSON{
		Name:           a.Name,
		Position:       a.Position,
		Inputs:         inputs,
		Start:          conditions,
		Goal:           goalToJSON(a.Goal),
		NodeType:       nodeType,
		Language:       a.Language,
		Content:        a.Content,
		PromptExecutor: a.PromptExecutor,
		Model:          a.Model,
		Temperature:    a.Temperature,
		OnError:        a.OnError,
		OnExhaustion:   a.OnExhaustion,
	}
}

func goalToJSON(goal *model.Goal) *GoalJSON {
	if goal == nil {
		return nil
	}
	return &GoalJSON{
		Objective:    goal.Objective,
		Validation:   goal.Validation,
		MaxTurns:     goal.MaxTurns,
		TokenBudget:  goal.TokenBudget,
		OnExhaustion: goal.OnExhaustion,
	}
}

func jsonToFlow(fj *FlowJSON) *model.Flow {
	agents := make([]model.Agent, len(fj.Agents))
	for i, aj := range fj.Agents {
		agents[i] = jsonToAgent(&aj)
	}
	return &model.Flow{
		Name:           fj.Name,
		Description:    fj.Description,
		ExternalInputs: fj.ExternalInputs,
		Defaults: model.Defaults{
			PromptExecutor: fj.Defaults.PromptExecutor,
			Model:          fj.Defaults.Model,
			Temperature:    fj.Defaults.Temperature,
		},
		Agents: agents,
	}
}

func jsonToAgent(aj *AgentJSON) model.Agent {
	inputs := make(map[string]model.Input)
	for k, v := range aj.Inputs {
		inputs[k] = model.Input{From: v.From, Fallback: v.Fallback}
	}

	conditions := make([]model.Condition, len(aj.Start))
	for i, cj := range aj.Start {
		c := model.Condition{
			When:         model.StringOrList(cj.When),
			Contains:     cj.Contains,
			MaxRuns:      cj.MaxRuns,
			OnExhaustion: cj.OnExhaustion,
		}
		if cj.Always != nil {
			c.Always = &model.AlwaysCondition{MaxRuns: cj.Always.MaxRuns}
		}
		conditions[i] = c
	}

	nodeType := model.PromptNode
	if aj.NodeType == "function" {
		nodeType = model.FunctionNode
	}

	return model.Agent{
		Name:           aj.Name,
		Position:       aj.Position,
		Inputs:         inputs,
		Start:          conditions,
		Goal:           jsonToGoal(aj.Goal),
		NodeType:       nodeType,
		Language:       aj.Language,
		Content:        aj.Content,
		PromptExecutor: aj.PromptExecutor,
		Model:          aj.Model,
		Temperature:    aj.Temperature,
		OnError:        aj.OnError,
		OnExhaustion:   aj.OnExhaustion,
	}
}

func jsonToGoal(goal *GoalJSON) *model.Goal {
	if goal == nil {
		return nil
	}
	return &model.Goal{
		Objective:    goal.Objective,
		Validation:   goal.Validation,
		MaxTurns:     goal.MaxTurns,
		TokenBudget:  goal.TokenBudget,
		OnExhaustion: goal.OnExhaustion,
	}
}
