package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type statusFileOps struct {
	mkdirAll  func(string, os.FileMode) error
	writeFile func(string, []byte, os.FileMode) error
	rename    func(string, string) error
	remove    func(string) error
}

var osStatusFileOps = statusFileOps{
	mkdirAll:  os.MkdirAll,
	writeFile: os.WriteFile,
	rename:    os.Rename,
	remove:    os.Remove,
}

func writeStatusFile(dir string, content []byte, ops statusFileOps) error {
	if err := ops.mkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir status directory: %w", err)
	}
	temp := filepath.Join(dir, "status.js.tmp")
	if err := ops.writeFile(temp, content, 0644); err != nil {
		_ = ops.remove(temp)
		return fmt.Errorf("write temporary status file: %w", err)
	}
	if err := ops.rename(temp, filepath.Join(dir, "status.js")); err != nil {
		_ = ops.remove(temp)
		return fmt.Errorf("rename status file: %w", err)
	}
	return nil
}

// statusDiagnosticReporter writes directly to the bounded process logger,
// never through logEvent/recentLogs or the status file it diagnoses.
type statusDiagnosticReporter struct {
	mu         sync.Mutex
	interval   time.Duration
	last       time.Time
	suppressed uint64
}

func (r *statusDiagnosticReporter) report(now time.Time, stage string, err error, emit func(string, ...any)) {
	if err == nil {
		return
	}
	r.mu.Lock()
	if !r.last.IsZero() && now.Sub(r.last) < r.interval {
		r.suppressed++
		r.mu.Unlock()
		return
	}
	suppressed := r.suppressed
	r.suppressed = 0
	r.last = now
	r.mu.Unlock()

	detail := strings.ToValidUTF8(err.Error(), "?")
	if len(detail) > 160 {
		detail = detail[:157] + "..."
	}
	emit("[status-write] %s failed: %s (suppressed %d similar failures)", stage, detail, suppressed)
}

const statusDiagnosticInterval = time.Minute

var statusWriteDiagnostics = statusDiagnosticReporter{interval: statusDiagnosticInterval}

func reportStatusWriteFailure(stage string, err error) {
	statusWriteDiagnostics.report(time.Now(), stage, err, log.Printf)
}
