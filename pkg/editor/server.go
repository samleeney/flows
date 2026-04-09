// Package editor provides the HTTP/WebSocket server for the visual flow editor.
package editor

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/samleeney/flows/pkg/model"
	"github.com/samleeney/flows/pkg/parser"
	"github.com/samleeney/flows/pkg/serializer"
)

// Server is the editor backend.
type Server struct {
	filePath string
	mu       sync.RWMutex
	flow     *model.Flow
	clients  map[*websocket.Conn]bool
	clientMu sync.Mutex
	upgrader websocket.Upgrader
	watcher  *FileWatcher
}

// NewServer creates a new editor server for the given flow file.
func NewServer(filePath string) (*Server, error) {
	flow, err := parser.ParseFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filePath, err)
	}

	s := &Server{
		filePath: filePath,
		flow:     flow,
		clients:  make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	return s, nil
}

// FlowJSON is the JSON representation sent to the frontend.
type FlowJSON struct {
	Name           string      `json:"name"`
	Description    string      `json:"description"`
	ExternalInputs []string    `json:"external_inputs"`
	Agents         []AgentJSON `json:"agents"`
}

// AgentJSON is the JSON representation of an agent for the frontend.
type AgentJSON struct {
	Name         string                 `json:"name"`
	Position     [2]int                 `json:"position"`
	Inputs       map[string]InputJSON   `json:"inputs"`
	Start        []ConditionJSON        `json:"start"`
	NodeType     string                 `json:"node_type"`
	Language     string                 `json:"language,omitempty"`
	Content      string                 `json:"content"`
	Model        string                 `json:"model,omitempty"`
	Temperature  float64                `json:"temperature,omitempty"`
	OnError      string                 `json:"on_error,omitempty"`
	OnExhaustion string                 `json:"on_exhaustion,omitempty"`
}

// InputJSON is the JSON representation of an input.
type InputJSON struct {
	From     string `json:"from"`
	Fallback string `json:"fallback,omitempty"`
}

// ConditionJSON is the JSON representation of a start condition.
type ConditionJSON struct {
	Always       *AlwaysJSON `json:"always,omitempty"`
	When         []string    `json:"when,omitempty"`
	Contains     string      `json:"contains,omitempty"`
	MaxRuns      int         `json:"max_runs,omitempty"`
	OnExhaustion string      `json:"on_exhaustion,omitempty"`
}

// AlwaysJSON is the JSON representation of an always condition.
type AlwaysJSON struct {
	MaxRuns int `json:"max_runs"`
}

// WSMessage is a WebSocket message envelope.
type WSMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Handler returns an http.Handler for the editor.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/flow", s.handleFlow)
	mux.HandleFunc("/ws", s.handleWebSocket)
	// Static files will be served from embedded FS in the future.
	// For now, serve a minimal HTML page.
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

func (s *Server) handleFlow(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		flowJSON := flowToJSON(s.flow)
		s.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(flowJSON)

	case http.MethodPut:
		var flowJSON FlowJSON
		if err := json.NewDecoder(r.Body).Decode(&flowJSON); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		flow := jsonToFlow(&flowJSON)

		s.mu.Lock()
		s.flow = flow
		s.mu.Unlock()

		// Write back to file
		if err := s.writeFile(flow); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Notify other clients
		s.broadcastFlow(flow)

		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade: %v", err)
		return
	}

	s.clientMu.Lock()
	s.clients[conn] = true
	s.clientMu.Unlock()

	// Send current state
	s.mu.RLock()
	flowJSON := flowToJSON(s.flow)
	s.mu.RUnlock()

	data, _ := json.Marshal(flowJSON)
	msg := WSMessage{Type: "flow", Data: data}
	conn.WriteJSON(msg)

	// Read messages from client
	for {
		var wsMsg WSMessage
		if err := conn.ReadJSON(&wsMsg); err != nil {
			break
		}

		switch wsMsg.Type {
		case "update_flow":
			var flowJSON FlowJSON
			if err := json.Unmarshal(wsMsg.Data, &flowJSON); err != nil {
				log.Printf("invalid flow update: %v", err)
				continue
			}

			flow := jsonToFlow(&flowJSON)

			s.mu.Lock()
			s.flow = flow
			s.mu.Unlock()

			if err := s.writeFile(flow); err != nil {
				log.Printf("write file: %v", err)
			}

			s.broadcastFlowExcept(flow, conn)
		}
	}

	s.clientMu.Lock()
	delete(s.clients, conn)
	s.clientMu.Unlock()
	conn.Close()
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<!DOCTYPE html>
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
	return os.WriteFile(s.filePath, data, 0o644)
}

func (s *Server) broadcastFlow(flow *model.Flow) {
	s.broadcastFlowExcept(flow, nil)
}

func (s *Server) broadcastFlowExcept(flow *model.Flow, exclude *websocket.Conn) {
	flowJSON := flowToJSON(flow)
	data, _ := json.Marshal(flowJSON)
	msg := WSMessage{Type: "flow", Data: data}

	s.clientMu.Lock()
	defer s.clientMu.Unlock()

	for conn := range s.clients {
		if conn == exclude {
			continue
		}
		if err := conn.WriteJSON(msg); err != nil {
			conn.Close()
			delete(s.clients, conn)
		}
	}
}

// StartFileWatcher begins watching the flow file for external changes.
func (s *Server) StartFileWatcher() error {
	watcher, err := NewFileWatcher(s.filePath, func() {
		flow, err := parser.ParseFile(s.filePath)
		if err != nil {
			log.Printf("reparse on file change: %v", err)
			return
		}
		s.mu.Lock()
		s.flow = flow
		s.mu.Unlock()
		s.broadcastFlow(flow)
	})
	if err != nil {
		return err
	}
	s.watcher = watcher
	return nil
}

// Close shuts down the server and file watcher.
func (s *Server) Close() {
	if s.watcher != nil {
		s.watcher.Close()
	}
	s.clientMu.Lock()
	for conn := range s.clients {
		conn.Close()
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
		Agents:         agents,
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
		Name:         a.Name,
		Position:     a.Position,
		Inputs:       inputs,
		Start:        conditions,
		NodeType:     nodeType,
		Language:     a.Language,
		Content:      a.Content,
		Model:        a.Model,
		Temperature:  a.Temperature,
		OnError:      a.OnError,
		OnExhaustion: a.OnExhaustion,
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
		Agents:         agents,
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
		Name:         aj.Name,
		Position:     aj.Position,
		Inputs:       inputs,
		Start:        conditions,
		NodeType:     nodeType,
		Language:     aj.Language,
		Content:      aj.Content,
		Model:        aj.Model,
		Temperature:  aj.Temperature,
		OnError:      aj.OnError,
		OnExhaustion: aj.OnExhaustion,
	}
}
