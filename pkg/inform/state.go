package inform

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	ErrStateCorrupted = errors.New("state file is corrupted or schema mismatch")
)

type SessionState struct {
	Schema     int        `json:"schema"`
	MAC        string     `json:"mac"`
	Adopted    bool       `json:"adopted"`
	Mode       CipherMode `json:"mode"`
	AuthKeyHex string     `json:"authkey,omitempty"`
	CfgVersion string     `json:"cfgversion,omitempty"`
	Paused     bool       `json:"paused,omitempty"`
}

type StateManager struct {
	mu        sync.Mutex
	statePath string
	mac       string
	state     SessionState
}

func NewStateManager(statePath string, mac string) (*StateManager, error) {
	normMAC, err := NormalizeMAC(mac)
	if err != nil {
		return nil, err
	}

	sm := &StateManager{
		statePath: statePath,
		mac:       normMAC,
		state: SessionState{
			Schema:  1,
			MAC:     normMAC,
			Adopted: false,
			Mode:    ModeCBC,
		},
	}

	if err := sm.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	return sm, nil
}

func (sm *StateManager) Load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	candidates := []string{sm.statePath, sm.statePath + ".bak"}
	var primaryErr error
	for i, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			var loaded SessionState
			loaded, err = decodeSessionState(data, sm.mac)
			if err == nil {
				sm.state = loaded
				return nil
			}
		}
		if i == 0 {
			primaryErr = err
		}
		// A missing primary with a corrupt/unreadable backup must not bootstrap.
		if i == 1 && errors.Is(primaryErr, os.ErrNotExist) && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return primaryErr
}

func decodeSessionState(data []byte, mac string) (SessionState, error) {
	var loaded SessionState
	if json.Unmarshal(data, &loaded) != nil || loaded.Schema != 1 || loaded.MAC != mac {
		return SessionState{}, ErrStateCorrupted
	}
	if loaded.AuthKeyHex != "" {
		if !validAuthKey(loaded.AuthKeyHex) {
			return SessionState{}, ErrStateCorrupted
		}
		loaded.Adopted = true
		loaded.Mode = ModeGCM
	} else {
		if loaded.Adopted {
			return SessionState{}, ErrStateCorrupted
		}
		loaded.Mode = ModeCBC
	}
	return loaded, nil
}

// atomicStateWrite never follows an existing temporary-file symlink and always
// replaces permissive destination files with a private, fully flushed file.
func atomicStateWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".inform-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func (sm *StateManager) Save() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	dir := filepath.Dir(sm.statePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sm.state, "", "  ")
	if err != nil {
		return err
	}
	// Retain a validated previous generation, never a corrupt primary. When
	// recovering from backup, leave that backup intact until the primary commits.
	prior, readErr := os.ReadFile(sm.statePath)
	if readErr == nil {
		if _, err := decodeSessionState(prior, sm.mac); err == nil {
			if err := atomicStateWrite(sm.statePath+".bak", prior); err != nil {
				return err
			}
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err := atomicStateWrite(sm.statePath, data); err != nil {
		return err
	}
	df, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer df.Close()
	return df.Sync()
}

func (sm *StateManager) GetKey() []byte {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state.Adopted && sm.state.AuthKeyHex != "" {
		if key, err := hex.DecodeString(sm.state.AuthKeyHex); err == nil && len(key) == 16 {
			return key
		}
	}
	return DefaultBootstrapKey[:]
}

func (sm *StateManager) GetMode() CipherMode {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.state.Adopted {
		return ModeGCM
	}
	return ModeCBC
}

func (sm *StateManager) IsAdopted() bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.state.Adopted
}

func (sm *StateManager) GetCfgVersion() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.state.CfgVersion == "" {
		return "0"
	}
	return sm.state.CfgVersion
}

// ApplyControllerResponse processes setparam, noop, or reset commands from the controller.
func (sm *StateManager) ApplyControllerResponse(payload map[string]any) (changed bool, err error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	respType, _ := payload["_type"].(string)

	switch respType {
	case "setparam", "setstate":
		params := make(map[string]string)
		if mgmtCfg, ok := payload["mgmt_cfg"].(string); ok {
			for _, line := range strings.Split(mgmtCfg, "\n") {
				line = strings.TrimSpace(line)
				if idx := strings.IndexByte(line, '='); idx > 0 {
					k := strings.TrimSpace(line[:idx])
					v := strings.TrimSpace(line[idx+1:])
					params[k] = v
				}
			}
		}
		for _, k := range []string{"authkey", "x_authkey", "cfgversion", "use_aes_gcm"} {
			if v, ok := payload[k].(string); ok && v != "" {
				params[k] = v
			}
		}

		keyHex := params["authkey"]
		if keyHex == "" {
			keyHex = params["x_authkey"]
		}
		if keyHex != "" && !validAuthKey(keyHex) {
			return false, fmt.Errorf("invalid controller auth key")
		}
		if validAuthKey(keyHex) {
			if sm.state.AuthKeyHex != keyHex || !sm.state.Adopted {
				sm.state.AuthKeyHex = keyHex
				sm.state.Adopted = true
				sm.state.Mode = ModeGCM
				changed = true
			}
		}

		if cfgVer, ok := params["cfgversion"]; ok && cfgVer != "" {
			if sm.state.CfgVersion != cfgVer {
				sm.state.CfgVersion = cfgVer
				changed = true
			}
		}

	case "cmd":
		if cmd, _ := payload["cmd"].(string); cmd == "set-adopt" {
			if keyHex, ok := payload["key"].(string); ok {
				if !validAuthKey(keyHex) {
					return false, fmt.Errorf("invalid controller auth key")
				}
				if sm.state.AuthKeyHex != keyHex || !sm.state.Adopted {
					sm.state.AuthKeyHex = keyHex
					sm.state.Adopted = true
					sm.state.Mode = ModeGCM
					changed = true
				}
			}
		}

	case "reset":
		sm.state.Adopted = false
		sm.state.AuthKeyHex = ""
		sm.state.Mode = ModeCBC
		sm.state.CfgVersion = "0"
		sm.state.Paused = true
		changed = true

	case "noop":
		// Standard heartbeat acknowledgement
	}

	return changed, nil
}

// Reset clears adoption status and keys.
func (sm *StateManager) Reset() error {
	sm.mu.Lock()
	sm.state = SessionState{
		Schema:  1,
		MAC:     sm.mac,
		Adopted: false,
		Mode:    ModeCBC,
	}
	sm.mu.Unlock()
	return sm.Save()
}

func validAuthKey(value string) bool {
	key, err := hex.DecodeString(value)
	return err == nil && len(key) == 16
}
