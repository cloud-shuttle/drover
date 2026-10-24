// Package sessionevent writes the issue-run session JSONL schema (ADR-003).
// Field names match drover-code/pkg/sessionevent. Keep the two copies in step.
// One JSON object per line.
package sessionevent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const Version = 1

const (
	TypeStarted           = "session.started"
	TypeStatus            = "session.status"
	TypeMessage           = "message.appended"
	TypeToolRequested     = "tool.requested"
	TypeToolFinished      = "tool.finished"
	TypeApprovalRequested = "approval.requested"
	TypeApprovalResolved  = "approval.resolved"
	TypeArtifactWritten   = "artifact.written"
	TypeLinked            = "session.linked"
)

// Event is one session-event line.
type Event struct {
	V         int            `json:"v"`
	Type      string         `json:"type"`
	TS        string         `json:"ts"`
	SessionID string         `json:"session_id"`
	IssueID   string         `json:"issue_id,omitempty"`
	Phase     string         `json:"phase,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

var fileMu sync.Map // path -> *sync.Mutex

// New builds an event stamped at the current UTC time.
func New(typ, sessionID string, data map[string]any) Event {
	return Event{
		V:         Version,
		Type:      typ,
		TS:        time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		Data:      data,
	}
}

// Append writes one event as a single JSON line. Creates parent directories.
func Append(path string, ev Event) error {
	if ev.V == 0 {
		ev.V = Version
	}
	if ev.TS == "" {
		ev.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}
	mu := mutexFor(path)
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(ev)
}

// ReadAll parses every line. A short line count is the caller's problem.
func ReadAll(path string) ([]Event, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Event
	dec := json.NewDecoder(bytes.NewReader(b))
	for dec.More() {
		var ev Event
		if err := dec.Decode(&ev); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

func mutexFor(path string) *sync.Mutex {
	v, _ := fileMu.LoadOrStore(path, &sync.Mutex{})
	return v.(*sync.Mutex)
}
