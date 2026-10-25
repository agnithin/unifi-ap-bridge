package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agnithin/unifi-ap-bridge/pkg/inform"
)

func TestAdoptionRecoveryRequiresConsecutiveRejections(t *testing.T) {
	for _, interruption := range []error{nil, errors.New("timeout"), &inform.HTTPError{StatusCode: 400}, &inform.HTTPError{StatusCode: 503}} {
		h := &exchangeHealth{}
		reject := &inform.HTTPError{StatusCode: 401}
		if h.observePost(reject) || h.observePost(reject) {
			t.Fatal("premature reset")
		}
		if h.observePost(interruption) {
			t.Fatal("unrelated failure reset identity")
		}
		if h.observePost(reject) || h.observePost(&inform.HTTPError{StatusCode: 404}) {
			t.Fatal("sequence did not restart")
		}
		if !h.observePost(reject) {
			t.Fatal("third consecutive rejection did not recover")
		}
		if h.observePost(reject) {
			t.Fatal("counter did not reset")
		}
	}
}

func TestHeartbeatOnlyReflectsSuccess(t *testing.T) {
	h := &exchangeHealth{LastAttempt: time.Now()}
	if h.lastSuccessString() != "" {
		t.Fatal("attempt reported as success")
	}
	h.LastSuccess = time.Date(2026, 10, 24, 22, 0, 0, 0, time.UTC)
	h.LastAttempt = h.LastSuccess.Add(time.Minute)
	h.observePost(errors.New("timeout"))
	if h.lastSuccessString() != "2026-10-24 22:00:00" {
		t.Fatal("failure changed last success")
	}
}

func TestConfigNormalizesInvalidInterval(t *testing.T) {
	for _, raw := range []string{`{"interval_sec":0}`, `{"interval_sec":-10}`, `{}`} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := loadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.IntervalSec != 10 {
			t.Fatalf("interval %d", cfg.IntervalSec)
		}
	}
}
