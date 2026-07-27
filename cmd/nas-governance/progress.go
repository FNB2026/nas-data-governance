package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/privatefs"
)

// progressSnapshot deliberately contains aggregate counters only. Source
// paths, names, hashes, and error strings are excluded so the file is safe to
// inspect while a long private scan is running.
type progressSnapshot struct {
	Command   string         `json:"command"`
	Stage     string         `json:"stage"`
	Status    string         `json:"status"`
	Processed int64          `json:"processed"`
	Total     int64          `json:"total,omitempty"`
	Failed    int64          `json:"failed,omitempty"`
	Reused    int64          `json:"reused,omitempty"`
	Details   map[string]int `json:"details,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type progressReporter struct {
	cancel   context.CancelFunc
	done     chan struct{}
	once     sync.Once
	path     string
	snapshot func() progressSnapshot
}

func progressStatus(stage string) string {
	if stage == "completed" {
		return "completed"
	}
	return "running"
}

func startProgressReporter(path string, interval time.Duration, snapshot func() progressSnapshot) (*progressReporter, error) {
	if path == "" {
		return nil, nil
	}
	if interval < time.Second || interval > 10*time.Minute {
		return nil, fmt.Errorf("--progress-interval must be between 1s and 10m")
	}
	if err := privatefs.EnsureDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &progressReporter{cancel: cancel, done: make(chan struct{}), path: path, snapshot: snapshot}
	if err := r.write(); err != nil {
		cancel()
		return nil, err
	}
	go func() {
		defer close(r.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = r.write()
			}
		}
	}()
	return r, nil
}

func (r *progressReporter) Stop() error {
	if r == nil {
		return nil
	}
	r.once.Do(func() {
		r.cancel()
		<-r.done
	})
	return r.write()
}

func (r *progressReporter) write() error {
	snapshot := r.snapshot()
	snapshot.UpdatedAt = time.Now().UTC()
	tmp := r.path + ".tmp"
	f, err := privatefs.Create(tmp)
	if err != nil {
		return err
	}
	encErr := json.NewEncoder(f).Encode(snapshot)
	closeErr := f.Close()
	if encErr != nil {
		return encErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmp, r.path); err != nil {
		return fmt.Errorf("replace progress snapshot: %w", err)
	}
	return privatefs.SecureFile(r.path)
}
