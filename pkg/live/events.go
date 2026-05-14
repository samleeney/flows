// Package live defines the event protocol and transport between `flow run`
// and a running `flow chart` editor.
package live

import "time"

const ProtocolVersion = 1

type EventKind string

const (
	KindRunStarted    EventKind = "run_started"
	KindAgentStarted  EventKind = "agent_started"
	KindAgentFinished EventKind = "agent_finished"
	KindRunFinished   EventKind = "run_finished"
)

type AgentStatus string

const (
	StatusDone   AgentStatus = "done"
	StatusFailed AgentStatus = "failed"
)

// EventEnvelope is the wire format for a single live event. Unused fields
// for a given Kind are omitted via json:"omitempty".
type EventEnvelope struct {
	Version int       `json:"version"`
	Kind    EventKind `json:"kind"`
	FlowKey string    `json:"flow_key"`
	RunID   string    `json:"run_id"`
	Seq     uint64    `json:"seq"`
	TS      time.Time `json:"ts"`

	Agent           string      `json:"agent,omitempty"`
	Iter            int         `json:"iter,omitempty"`
	Status          AgentStatus `json:"status,omitempty"`
	DurationMS      int64       `json:"duration_ms,omitempty"`
	OutputPreview   string      `json:"output_preview,omitempty"`
	OutputBytes     int         `json:"output_bytes,omitempty"`
	OutputTruncated bool        `json:"output_truncated,omitempty"`
	Error           string      `json:"error,omitempty"`
	OK              *bool       `json:"ok,omitempty"`
}

// Observer is the runtime-facing event sink. Publish is fire-and-forget and
// MUST never block on network I/O. Close drains queued events best-effort.
type Observer interface {
	Publish(EventEnvelope) error
	Close() error
}

// NopObserver discards every event. Used when no editor is reachable.
type NopObserver struct{}

func (NopObserver) Publish(EventEnvelope) error { return nil }
func (NopObserver) Close() error                { return nil }
