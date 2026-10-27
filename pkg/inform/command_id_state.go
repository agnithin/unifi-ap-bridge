package inform

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var errCommandIDStorePathRequired = errors.New("persistent reboot command ID state path required")

type commandIDStateFile struct {
	Schema int      `json:"schema"`
	IDs    []string `json:"ids"`
}

func commandRequestID(value any) ([32]byte, bool) {
	s, ok := value.(string)
	if !ok || len(s) == 0 || len(s) > 128 {
		return [32]byte{}, false
	}
	return sha256.Sum256([]byte(s)), true
}

func (d *RebootDispatcher) loadIDs() error {
	if d.stateLoaded {
		return d.stateErr
	}
	d.stateLoaded = true
	if d.statePath == "" {
		d.stateErr = errCommandIDStorePathRequired
		return d.stateErr
	}

	info, err := os.Lstat(d.statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		d.stateErr = errors.New("cannot read reboot command ID state")
		return d.stateErr
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		d.stateErr = errors.New("invalid reboot command ID state file")
		return d.stateErr
	}
	raw, err := os.ReadFile(d.statePath)
	if err != nil || len(raw) > 8192 {
		d.stateErr = errors.New("cannot read reboot command ID state")
		return d.stateErr
	}
	var state commandIDStateFile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil || state.Schema != 1 || len(state.IDs) > maxRememberedCommandIDs {
		d.stateErr = errors.New("invalid reboot command ID state")
		return d.stateErr
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		d.stateErr = errors.New("invalid reboot command ID state")
		return d.stateErr
	}
	d.idOrder = make([][32]byte, 0, len(state.IDs))
	d.ids = make(map[[32]byte]struct{}, len(state.IDs))
	for _, encoded := range state.IDs {
		if len(encoded) != sha256.Size*2 || strings.ToLower(encoded) != encoded {
			d.stateErr = errors.New("invalid reboot command ID state")
			return d.stateErr
		}
		decoded, err := hex.DecodeString(encoded)
		if err != nil {
			d.stateErr = errors.New("invalid reboot command ID state")
			return d.stateErr
		}
		var id [32]byte
		copy(id[:], decoded)
		if _, duplicate := d.ids[id]; duplicate {
			d.stateErr = errors.New("invalid reboot command ID state")
			return d.stateErr
		}
		d.ids[id] = struct{}{}
		d.idOrder = append(d.idOrder, id)
	}
	return nil
}

func writeCommandIDState(path string, ids [][32]byte) error {
	if path == "" || len(ids) > maxRememberedCommandIDs {
		return errors.New("invalid reboot command ID state path or size")
	}
	state := commandIDStateFile{Schema: 1, IDs: make([]string, 0, len(ids))}
	seen := make(map[[32]byte]struct{}, len(ids))
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			return errors.New("duplicate reboot command ID")
		}
		seen[id] = struct{}{}
		state.IDs = append(state.IDs, hex.EncodeToString(id[:]))
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".controller-command-state-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(encoded)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync reboot command ID state directory: %w", err)
	}
	return nil
}
