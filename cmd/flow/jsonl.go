package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/samleeney/flows/pkg/clioutput"
	"github.com/samleeney/flows/pkg/live"
	"github.com/samleeney/flows/pkg/runtime"
	"github.com/spf13/cobra"
)

type jsonlObserver struct {
	mu       sync.Mutex
	encoder  *json.Encoder
	writeErr error
	terminal *live.EventEnvelope
}

func newJSONLObserver(writer io.Writer) *jsonlObserver {
	return &jsonlObserver{encoder: json.NewEncoder(writer)}
}

func (o *jsonlObserver) Publish(envelope live.EventEnvelope) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if envelope.Kind == live.KindRunFinished {
		copy := envelope
		o.terminal = &copy
		return nil
	}
	if o.writeErr != nil {
		return o.writeErr
	}
	event := clioutput.Event{
		SchemaVersion: clioutput.SchemaVersion,
		Command:       "run",
		OK:            envelope.Status != live.StatusFailed,
		RunID:         envelope.RunID,
		Sequence:      envelope.Seq,
		Timestamp:     envelope.TS,
		Block:         envelope.Agent,
		Iteration:     envelope.Iter,
		Status:        string(envelope.Status),
		DurationMS:    envelope.DurationMS,
		OutputPreview: envelope.OutputPreview,
	}
	switch envelope.Kind {
	case live.KindRunStarted:
		event.Event = "run_started"
	case live.KindAgentStarted:
		event.Event = "block_started"
	case live.KindAgentFinished:
		event.Event = "block_finished"
		if envelope.Error != "" {
			event.Error = &clioutput.Error{Code: "block_failed", Message: envelope.Error}
		}
	default:
		return nil
	}
	if err := o.encoder.Encode(event); err != nil {
		o.writeErr = fmt.Errorf("write JSONL event: %w", err)
		return o.writeErr
	}
	return nil
}

func (o *jsonlObserver) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.writeErr
}

func (o *jsonlObserver) finish(result *runtime.RunResult, outputs []clioutput.Output, runErr error, errorCode string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.writeErr != nil {
		return o.writeErr
	}
	if o.terminal == nil {
		return errors.New("runtime did not emit a terminal event")
	}
	sequence := o.terminal.Seq
	timestamp := o.terminal.TS
	runID := o.terminal.RunID
	if runErr != nil {
		executionEvent := clioutput.Event{
			SchemaVersion: clioutput.SchemaVersion,
			Command:       "run",
			OK:            false,
			Event:         "execution_error",
			RunID:         runID,
			Sequence:      sequence,
			Timestamp:     timestamp,
			Error:         &clioutput.Error{Code: errorCode, Message: runErr.Error()},
		}
		if err := o.encoder.Encode(executionEvent); err != nil {
			return fmt.Errorf("write JSONL execution error: %w", err)
		}
		sequence++
		timestamp = time.Now().UTC()
	}
	terminal := clioutput.Event{
		SchemaVersion: clioutput.SchemaVersion,
		Command:       "run",
		OK:            runErr == nil,
		Event:         "run_finished",
		RunID:         runID,
		Sequence:      sequence,
		Timestamp:     timestamp,
		Outputs:       &outputs,
	}
	if runErr != nil {
		terminal.Error = &clioutput.Error{Code: errorCode, Message: runErr.Error()}
	}
	if result != nil && result.RunID != "" {
		terminal.RunID = result.RunID
	}
	if err := o.encoder.Encode(terminal); err != nil {
		return fmt.Errorf("write JSONL terminal event: %w", err)
	}
	return nil
}

func failJSONL(cmd *cobra.Command, code, message string, diagnostics []clioutput.Diagnostic, exitCode int) error {
	runID, err := live.NewRunID()
	if err != nil {
		return &commandError{err: fmt.Errorf("generate run id: %w", err), exitCode: exitInternal}
	}
	event := clioutput.Event{
		SchemaVersion: clioutput.SchemaVersion,
		Command:       "run",
		OK:            false,
		Event:         "execution_error",
		RunID:         runID,
		Sequence:      1,
		Timestamp:     time.Now().UTC(),
		Error:         &clioutput.Error{Code: code, Message: message, Diagnostics: diagnostics},
	}
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(event); err != nil {
		return &commandError{err: fmt.Errorf("write JSONL error: %w", err), exitCode: exitInternal}
	}
	return &commandError{err: errors.New(message), exitCode: exitCode, emitted: true}
}
