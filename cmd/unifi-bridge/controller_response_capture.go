package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agnithin/unifi-ap-bridge/pkg/inform"
)

const (
	maxControllerResponseCaptures = 3
	maxControllerResponseBytes    = inform.MaxJSONSize
	controllerResponseCaptureDir  = "/tmp/unifi-bridge/controller-probe"
)

var errControllerResponseCapture = errors.New("controller response capture failed")

type controllerResponseCapture struct {
	mu        sync.Mutex
	dir       string
	remaining int
}

// capture is called only after UnpackInform has authenticated and decoded the
// response. Both packet modes and the outgoing exchange must be GCM, and the
// AP must have been adopted before this exchange began.
func (c *controllerResponseCapture) capture(adopted bool, key []byte, requestMode, responseMode inform.CipherMode, responseType any, rawJSON []byte) error {
	if c == nil || !adopted || len(key) != 16 || bytes.Equal(key, inform.DefaultBootstrapKey[:]) || requestMode != inform.ModeGCM || responseMode != inform.ModeGCM || responseType == "noop" {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.remaining <= 0 {
		return nil
	}
	// Spend the finite response budget before any I/O, including oversize or
	// filesystem failures, so repeated responses cannot trigger retries.
	c.remaining--
	if len(rawJSON) == 0 || len(rawJSON) > maxControllerResponseBytes || c.dir == "" {
		return errControllerResponseCapture
	}
	if err := ensureControllerCaptureDir(c.dir); err != nil {
		return errControllerResponseCapture
	}

	f, err := os.CreateTemp(c.dir, "response-*.json")
	if err != nil {
		return errControllerResponseCapture
	}
	name := f.Name()
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if err := f.Chmod(0600); err != nil {
		return errControllerResponseCapture
	}
	if _, err := f.Write(rawJSON); err != nil {
		return errControllerResponseCapture
	}
	if err := f.Close(); err != nil {
		return errControllerResponseCapture
	}
	ok = true
	return nil
}

func ensureControllerCaptureDir(dir string) error {
	clean := filepath.Clean(dir)
	if !filepath.IsAbs(clean) || clean == string(filepath.Separator) {
		return fmt.Errorf("invalid capture directory")
	}
	if strings.HasPrefix(clean, "/tmp/") {
		if err := verifyCaptureDirectory("/tmp"); err != nil {
			return err
		}
	}
	parent := filepath.Dir(clean)
	if err := ensureCaptureDirectory(parent, 0700); err != nil {
		return err
	}
	return ensureCaptureDirectory(clean, 0700)
}

func verifyCaptureDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("capture path is not a directory")
	}
	return nil
}

func ensureCaptureDirectory(path string, mode os.FileMode) error {
	if err := os.Mkdir(path, mode); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("capture path is not a directory")
	}
	if info.Mode().Perm() != mode.Perm() {
		return os.Chmod(path, mode.Perm())
	}
	return nil
}
