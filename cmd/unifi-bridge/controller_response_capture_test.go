package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agnithin/unifi-ap-bridge/pkg/inform"
)

func TestControllerResponseCaptureGates(t *testing.T) {
	for _, tc := range []struct {
		name         string
		adopted      bool
		requestMode  inform.CipherMode
		responseMode inform.CipherMode
		responseType any
	}{
		{name: "bootstrap", requestMode: inform.ModeCBC, responseMode: inform.ModeCBC, responseType: "cmd"},
		{name: "not adopted", requestMode: inform.ModeGCM, responseMode: inform.ModeGCM, responseType: "cmd"},
		{name: "request was not gcm", adopted: true, requestMode: inform.ModeCBC, responseMode: inform.ModeGCM, responseType: "cmd"},
		{name: "response was not gcm", adopted: true, requestMode: inform.ModeGCM, responseMode: inform.ModeCBC, responseType: "cmd"},
		{name: "noop", adopted: true, requestMode: inform.ModeGCM, responseMode: inform.ModeGCM, responseType: "noop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &controllerResponseCapture{dir: t.TempDir() + "/probe", remaining: 3}
			if err := capture.capture(tc.adopted, []byte("0123456789abcdef"), tc.requestMode, tc.responseMode, tc.responseType, []byte(`{"_type":"cmd"}`)); err != nil {
				t.Fatalf("capture returned error: %v", err)
			}
			if capture.remaining != 3 {
				t.Fatalf("ineligible response consumed budget: %d", capture.remaining)
			}
			if _, err := os.Stat(capture.dir); !os.IsNotExist(err) {
				t.Fatalf("ineligible response created capture directory: %v", err)
			}
		})
	}
}

func TestControllerResponseCaptureBoundAndPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "probe")
	capture := &controllerResponseCapture{dir: dir, remaining: 3}
	for i := 0; i < 4; i++ {
		if err := capture.capture(true, []byte("0123456789abcdef"), inform.ModeGCM, inform.ModeGCM, "cmd", []byte(`{"_type":"cmd"}`)); err != nil {
			t.Fatalf("capture %d returned error: %v", i, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || capture.remaining != 0 {
		t.Fatalf("got %d captures with %d remaining", len(entries), capture.remaining)
	}
	if got := modePerm(t, dir); got != 0700 {
		t.Fatalf("capture dir mode = %04o, want 0700", got)
	}
	for _, entry := range entries {
		if got := modePerm(t, filepath.Join(dir, entry.Name())); got != 0600 {
			t.Fatalf("capture file mode = %04o, want 0600", got)
		}
	}
}

func TestControllerResponseCaptureOversizeConsumesBudgetWithoutFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "probe")
	capture := &controllerResponseCapture{dir: dir, remaining: 3}
	if err := capture.capture(true, []byte("0123456789abcdef"), inform.ModeGCM, inform.ModeGCM, "cmd", make([]byte, maxControllerResponseBytes+1)); err == nil {
		t.Fatal("oversize response was accepted")
	}
	if capture.remaining != 2 {
		t.Fatalf("oversize response did not consume budget: %d", capture.remaining)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("oversize response created capture directory: %v", err)
	}
}

func TestEnsureControllerCaptureDirRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "probe")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := ensureControllerCaptureDir(link); err == nil {
		t.Fatal("symlink capture directory was accepted")
	}
}

func TestLoadConfigControllerResponseCaptureCountBounds(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{value: `{"enabled":true}`, valid: true},
		{value: `{"controller_response_capture_count":0}`, valid: true},
		{value: `{"controller_response_capture_count":3}`, valid: true},
		{value: `{"controller_response_capture_count":-1}`},
		{value: `{"controller_response_capture_count":4}`},
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(tc.value), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := loadConfig(path)
		if (err == nil) != tc.valid {
			t.Fatalf("loadConfig(%s) error = %v, valid = %v", tc.value, err, tc.valid)
		}
		if err == nil && cfg.ControllerResponseCaptureCount < 0 {
			t.Fatalf("accepted invalid count %d", cfg.ControllerResponseCaptureCount)
		}
	}
}

func modePerm(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestControllerResponseCaptureRejectsBootstrapGCMKey(t *testing.T) {
	c := &controllerResponseCapture{dir: t.TempDir() + "/probe", remaining: 3}
	for _, key := range [][]byte{nil, inform.DefaultBootstrapKey[:]} {
		if err := c.capture(true, key, inform.ModeGCM, inform.ModeGCM, "cmd", []byte(`{"_type":"cmd"}`)); err != nil {
			t.Fatal(err)
		}
	}
	if c.remaining != 3 {
		t.Fatal("ineligible key consumed capture budget")
	}
	if _, err := os.Stat(c.dir); !os.IsNotExist(err) {
		t.Fatal("ineligible key created capture directory")
	}
}
