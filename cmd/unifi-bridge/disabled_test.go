package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDisabledStatusPreservesInstalledIdentity(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Enabled: false, Branch: "dev", Model: "U6IW", IntervalSec: 15, ControllerURL: "http://controller.test:8080/inform"}
	if e := publishDisabledStatus(dir, cfg); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "status.js"))
	body := strings.TrimSuffix(strings.SplitN(string(raw), "=", 2)[1], ";\n")
	var status map[string]any
	if e := json.Unmarshal([]byte(body), &status); e != nil {
		t.Fatal(e)
	}
	if status["enabled"] != false || status["running"] != false || status["branch"] != "dev" || status["model_code"] != "U6IW" || status["version"] != version {
		t.Fatal("lost disabled configuration identity")
	}
}
