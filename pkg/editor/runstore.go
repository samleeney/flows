package editor

import (
	"sync"
	"time"

	"github.com/samleeney/flows/pkg/live"
)

const (
	MaxRuns         = 10
	CompletedRunTTL = 1 * time.Hour
)

// AgentLiveState is the per-agent execution state held by the run store.
type AgentLiveState struct {
	Status          string `json:"status"`
	Iter            int    `json:"iter"`
	DurationMS      int64  `json:"duration_ms,omitempty"`
	OutputPreview   string `json:"output_preview,omitempty"`
	OutputBytes     int    `json:"output_bytes,omitempty"`
	OutputTruncated bool   `json:"output_truncated,omitempty"`
	Error           string `json:"error,omitempty"`
}

// RunRecord captures the live state of one `flow run` invocation.
type RunRecord struct {
	RunID          string                     `json:"run_id"`
	StartedAt      time.Time                  `json:"started_at"`
	FinishedAt     *time.Time                 `json:"finished_at,omitempty"`
	OK             *bool                      `json:"ok,omitempty"`
	Error          string                     `json:"error,omitempty"`
	Disconnected   bool                       `json:"disconnected"`
	LastSeq        uint64                     `json:"last_seq"`
	Agents         map[string]*AgentLiveState `json:"agents"`
	ExternalInputs []live.ExternalInputOrigin `json:"external_inputs,omitempty"`
}

// RunSnapshot is the WebSocket payload sent on connect and on retention sweeps.
type RunSnapshot struct {
	Version     int          `json:"version"`
	FlowKey     string       `json:"flow_key"`
	GeneratedAt time.Time    `json:"generated_at"`
	Runs        []*RunRecord `json:"runs"`
}

// RunStore is the editor's in-memory store of recent runs. Reads and writes
// are serialized with sync.RWMutex; Snapshot returns a deep copy.
type RunStore struct {
	flowKey string

	mu    sync.RWMutex
	runs  map[string]*RunRecord
	order []string // insertion order; oldest first
}

func NewRunStore(flowKey string) *RunStore {
	return &RunStore{
		flowKey: flowKey,
		runs:    make(map[string]*RunRecord),
	}
}

// Apply ingests a single event. Events with version != current protocol or
// seq <= the run's last_seq are ignored. Returns true if the event was
// applied (the caller can then broadcast it).
func (s *RunStore) Apply(env live.EventEnvelope) bool {
	if env.Version != live.ProtocolVersion {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, exists := s.runs[env.RunID]
	if !exists {
		if env.Kind != live.KindRunStarted {
			// First event for an unknown run that isn't run_started — accept it
			// and create the record. Could happen on partial delivery.
		}
		rec = &RunRecord{
			RunID:     env.RunID,
			StartedAt: env.TS,
			Agents:    make(map[string]*AgentLiveState),
		}
		s.runs[env.RunID] = rec
		s.order = append(s.order, env.RunID)
	}

	if env.Seq <= rec.LastSeq {
		return false
	}
	rec.LastSeq = env.Seq

	switch env.Kind {
	case live.KindRunStarted:
		rec.StartedAt = env.TS
		rec.ExternalInputs = copyExternalInputs(env.ExternalInputs)
	case live.KindAgentStarted:
		ag := rec.Agents[env.Agent]
		if ag == nil {
			ag = &AgentLiveState{}
			rec.Agents[env.Agent] = ag
		}
		ag.Status = "running"
		ag.Iter = env.Iter
	case live.KindAgentFinished:
		ag := rec.Agents[env.Agent]
		if ag == nil {
			ag = &AgentLiveState{}
			rec.Agents[env.Agent] = ag
		}
		ag.Iter = env.Iter
		ag.Status = string(env.Status)
		if ag.Status == "" {
			ag.Status = "done"
		}
		ag.DurationMS = env.DurationMS
		ag.OutputPreview = env.OutputPreview
		ag.OutputBytes = env.OutputBytes
		ag.OutputTruncated = env.OutputTruncated
		ag.Error = env.Error
	case live.KindRunFinished:
		ts := env.TS
		rec.FinishedAt = &ts
		rec.OK = env.OK
		rec.Error = env.Error
	}

	s.evictLocked()
	return true
}

// MarkDisconnected flags a run as disconnected (CLI died before run_finished).
// Idempotent.
func (s *RunStore) MarkDisconnected(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.runs[runID]
	if !ok || rec.FinishedAt != nil {
		return
	}
	rec.Disconnected = true
	now := time.Now().UTC()
	rec.FinishedAt = &now
}

// Snapshot returns a deep copy of the current store state.
func (s *RunStore) Snapshot() RunSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runs := make([]*RunRecord, 0, len(s.order))
	for _, id := range s.order {
		rec, ok := s.runs[id]
		if !ok {
			continue
		}
		runs = append(runs, deepCopyRun(rec))
	}
	return RunSnapshot{
		Version:     live.ProtocolVersion,
		FlowKey:     s.flowKey,
		GeneratedAt: time.Now().UTC(),
		Runs:        runs,
	}
}

func deepCopyRun(rec *RunRecord) *RunRecord {
	cp := *rec
	if rec.FinishedAt != nil {
		t := *rec.FinishedAt
		cp.FinishedAt = &t
	}
	if rec.OK != nil {
		v := *rec.OK
		cp.OK = &v
	}
	cp.Agents = make(map[string]*AgentLiveState, len(rec.Agents))
	for k, v := range rec.Agents {
		ag := *v
		cp.Agents[k] = &ag
	}
	cp.ExternalInputs = copyExternalInputs(rec.ExternalInputs)
	return &cp
}

func copyExternalInputs(inputs []live.ExternalInputOrigin) []live.ExternalInputOrigin {
	if len(inputs) == 0 {
		return nil
	}
	out := make([]live.ExternalInputOrigin, len(inputs))
	copy(out, inputs)
	return out
}

// evictLocked enforces MaxRuns and CompletedRunTTL. Active runs are pinned.
// Caller must hold s.mu.
func (s *RunStore) evictLocked() {
	now := time.Now()

	// Age out completed runs older than TTL.
	kept := s.order[:0]
	for _, id := range s.order {
		rec, ok := s.runs[id]
		if !ok {
			continue
		}
		if rec.FinishedAt != nil && now.Sub(*rec.FinishedAt) > CompletedRunTTL {
			delete(s.runs, id)
			continue
		}
		kept = append(kept, id)
	}
	s.order = kept

	// Cap completed runs at MaxRuns by LRU. Active runs are not counted
	// against the cap.
	completed := 0
	for _, id := range s.order {
		if rec, ok := s.runs[id]; ok && rec.FinishedAt != nil {
			completed++
		}
	}
	for completed > MaxRuns {
		// Find the oldest completed run and evict it.
		for i, id := range s.order {
			if rec, ok := s.runs[id]; ok && rec.FinishedAt != nil {
				delete(s.runs, id)
				s.order = append(s.order[:i], s.order[i+1:]...)
				completed--
				break
			}
		}
	}
}
