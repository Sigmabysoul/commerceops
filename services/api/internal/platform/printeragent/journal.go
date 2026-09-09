// This file persists completed job identifiers so agent restarts do not print the same job twice in the local printer-agent package.
package printeragent

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

type Journal struct {
	mu     sync.Mutex
	path   string
	states map[string]string
}
type journalEntry struct {
	JobID string    `json:"job_id"`
	State string    `json:"state"`
	At    time.Time `json:"at"`
}

func OpenJournal(path string) (*Journal, error) {
	if path == "" {
		return nil, errors.New("journal path is required")
	}
	j := &Journal{path: path, states: map[string]string{}}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return j, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		var e journalEntry
		if json.Unmarshal(scan.Bytes(), &e) == nil && e.JobID != "" {
			j.states[e.JobID] = e.State
		}
	}
	if err = scan.Err(); err != nil {
		return nil, err
	}
	return j, nil
}
func (j *Journal) State(id string) string { j.mu.Lock(); defer j.mu.Unlock(); return j.states[id] }
func (j *Journal) Record(id, state string) error {
	// Append and fsync before changing memory so a successful return means the
	// submission boundary survives a process or machine restart.
	if id == "" || state == "" {
		return errors.New("job and state are required")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	data, err := json.Marshal(journalEntry{JobID: id, State: state, At: time.Now().UTC()})
	if err == nil {
		_, err = f.Write(append(data, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	j.states[id] = state
	return nil
}
