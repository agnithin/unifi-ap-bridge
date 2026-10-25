package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogRemainsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := &boundedLogWriter{file: f, maxBytes: 32}
	for i := 0; i < 100; i++ {
		if _, err := w.Write([]byte("heartbeat ok\n")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Write([]byte("abcdefghijklmnopqrstuvwxyz0123456789")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 32 || string(b) != "efghijklmnopqrstuvwxyz0123456789" {
		t.Fatalf("unexpected retained log: %q", b)
	}
}
