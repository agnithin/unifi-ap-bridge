package telemetry

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"sync"
)

const (
	maxWLANMappingFileBytes = 64 * 1024
	maxWLANMappings         = 256
)

// WLANMappingCache reads an optional host-published exact SSID to legacy
// ObjectId file. Failed, malformed, oversized, or empty updates preserve the
// last valid entries. The caller should overlay manually configured mappings
// after Mappings so explicit values take precedence.
type WLANMappingCache struct {
	mu      sync.Mutex
	path    string
	lastMap map[string]string
	lastErr string
}

func NewWLANMappingCache(path string) *WLANMappingCache {
	return &WLANMappingCache{path: path, lastMap: make(map[string]string)}
}

// Mappings refreshes from path when the whole file is valid and returns a copy
// of the accumulated last-known-good map. A valid partial update replaces only
// the SSIDs it contains, preserving previously known entries through outages.
func (c *WLANMappingCache) Mappings() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.path != "" {
		mappings, status := readWLANMappingFile(c.path)
		c.lastErr = status
		if status == "" {
			updated := cloneWLANMapping(c.lastMap)
			for ssid, id := range mappings {
				updated[ssid] = id
			}
			if len(updated) <= maxWLANMappings {
				c.lastMap = updated
			} else {
				c.lastErr = "WLAN mapping cache limit exceeded"
			}
		}
	}
	return cloneWLANMapping(c.lastMap)
}

// LastError returns a fixed, sanitized refresh status without exposing file
// contents, SSIDs, or credentials. Empty means the most recent refresh passed.
func (c *WLANMappingCache) LastError() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}

func readWLANMappingFile(path string) (map[string]string, string) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "WLAN mapping file unavailable"
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxWLANMappingFileBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxWLANMappingFileBytes {
		return nil, "WLAN mapping file is empty, unreadable, or oversized"
	}
	mappings, valid := parseWLANMapping(raw)
	if !valid {
		return nil, "WLAN mapping file is invalid"
	}
	return mappings, ""
}

func parseWLANMapping(raw []byte) (map[string]string, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, false
	}
	mappings := make(map[string]string)
	for decoder.More() {
		token, err = decoder.Token()
		ssid, isString := token.(string)
		if err != nil || !isString || ssid == "" || len(ssid) > 32 {
			return nil, false
		}
		if _, duplicate := mappings[ssid]; duplicate {
			return nil, false
		}
		var id string
		if err := decoder.Decode(&id); err != nil || !ValidLegacyWlanID(id) {
			return nil, false
		}
		mappings[ssid] = id
		if len(mappings) > maxWLANMappings {
			return nil, false
		}
	}
	if _, err = decoder.Token(); err != nil || len(mappings) == 0 {
		return nil, false
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, false
	}
	return mappings, true
}

func cloneWLANMapping(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source))
	for ssid, id := range source {
		copy[ssid] = id
	}
	return copy
}
