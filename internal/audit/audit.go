// Package audit writes one immutable JSON line per decision. The point is
// that six months from now, someone can answer "why did this happen?"
// from this file alone, without guessing which version of the rules was
// live at the time.
package audit

import (
	"encoding/json"
	"os"
	"time"
)

// Entry represents a single auditable decision event.
type Entry struct {
	Timestamp            string            `json:"timestamp"`
	SessionID            string            `json:"session_id"`
	CustomerID           string            `json:"customer_id"`
	PolicyName           string            `json:"policy_name"`
	PolicyVersion        string            `json:"policy_version"`
	InputCategories      map[string]string `json:"input_categories"`
	AIAction             string            `json:"ai_action"`
	AIReason             string            `json:"ai_reason"`
	FinalAction          string            `json:"final_action"`
	OverriddenByHardRule bool              `json:"overridden_by_hard_rule"`
}

// Logger handles writing audit entries to an append-only JSONL file.
type Logger struct {
	path string
}

// New creates a new audit Logger writing to the specified path.
func New(path string) *Logger {
	return &Logger{path: path}
}

// Write appends one entry as a single JSON line (JSONL), so the file stays
// grep-able and streamable even at millions of rows.
func (l *Logger) Write(e Entry) error {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	enc := json.NewEncoder(f)
	return enc.Encode(e)
}
