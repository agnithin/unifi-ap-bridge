package telemetry

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWLANMappingCacheKeepsLastKnownGoodAndMergesPartialRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wlan-mapping.json")
	cache := NewWLANMappingCache(path)
	if got := cache.Mappings(); len(got) != 0 {
		t.Fatalf("missing file should start empty: %#v", got)
	}
	if err := os.WriteFile(path, []byte(`{"blueberry":"6adbb92027327fe7fefe9d60","blueberry-prime":"6adb775a27327fe7fefdc22c"}`), 0600); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"blueberry":       "6adbb92027327fe7fefe9d60",
		"blueberry-prime": "6adb775a27327fe7fefdc22c",
	}
	if got := cache.Mappings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("initial mapping: %#v", got)
	}
	if got := cache.LastError(); got != "" {
		t.Fatalf("valid refresh status: %q", got)
	}
	if err := os.WriteFile(path, []byte(`{"new-ssid":"aaaaaaaaaaaaaaaaaaaaaaaa"}`), 0600); err != nil {
		t.Fatal(err)
	}
	want["new-ssid"] = "aaaaaaaaaaaaaaaaaaaaaaaa"
	if got := cache.Mappings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("partial refresh cleared cached entries: %#v", got)
	}
	if err := os.WriteFile(path, []byte(`{"broken":"not-an-object-id"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := cache.Mappings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("invalid refresh replaced cached entries: %#v", got)
	}
	if got := cache.LastError(); got != "WLAN mapping file is invalid" {
		t.Fatalf("invalid refresh status: %q", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := cache.Mappings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("read failure cleared cached entries: %#v", got)
	}
	if got := cache.LastError(); got != "WLAN mapping file unavailable" {
		t.Fatalf("missing file status: %q", got)
	}
}

func TestWLANMappingCacheRejectsOversizedEmptyAndMalformedFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wlan-mapping.json")
	cache := NewWLANMappingCache(path)
	valid := []byte(`{"known":"0123456789abcdef01234567"}`)
	if err := os.WriteFile(path, valid, 0600); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"known": "0123456789abcdef01234567"}
	if got := cache.Mappings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("valid mapping: %#v", got)
	}
	for _, invalid := range [][]byte{
		{},
		[]byte(`{}`),
		[]byte(`{"known":"0123456789abcdef01234567","known":"aaaaaaaaaaaaaaaaaaaaaaaa"}`),
		[]byte(`{"known":"0123456789abcdef01234567"} trailing`),
		[]byte(strings.Repeat(" ", maxWLANMappingFileBytes+1)),
	} {
		if err := os.WriteFile(path, invalid, 0600); err != nil {
			t.Fatal(err)
		}
		if got := cache.Mappings(); !reflect.DeepEqual(got, want) {
			t.Fatalf("invalid file cleared or changed cache (%d bytes): %#v", len(invalid), got)
		}
	}
}

func TestWLANMappingCacheUnionLimitPreservesOldMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wlan-mapping.json")
	var contents strings.Builder
	contents.WriteByte('{')
	for i := 0; i < maxWLANMappings; i++ {
		if i > 0 {
			contents.WriteByte(',')
		}
		fmt.Fprintf(&contents, `"ssid-%03d":"0123456789abcdef01234567"`, i)
	}
	contents.WriteByte('}')
	if err := os.WriteFile(path, []byte(contents.String()), 0600); err != nil {
		t.Fatal(err)
	}
	cache := NewWLANMappingCache(path)
	if got := cache.Mappings(); len(got) != maxWLANMappings {
		t.Fatalf("initial mapping count: %d", len(got))
	}
	if err := os.WriteFile(path, []byte(`{"another-ssid":"aaaaaaaaaaaaaaaaaaaaaaaa"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := cache.Mappings(); len(got) != maxWLANMappings {
		t.Fatalf("over-limit update changed map size: %d", len(got))
	}
	if got := cache.LastError(); got != "WLAN mapping cache limit exceeded" {
		t.Fatalf("over-limit status: %q", got)
	}
}

func TestWLANMappingCacheReturnsDefensiveCopy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wlan-mapping.json")
	if err := os.WriteFile(path, []byte(`{"ssid":"0123456789abcdef01234567"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cache := NewWLANMappingCache(path)
	first := cache.Mappings()
	first["ssid"] = "aaaaaaaaaaaaaaaaaaaaaaaa"
	if got := cache.Mappings()["ssid"]; got != "0123456789abcdef01234567" {
		t.Fatalf("caller changed cache through returned map: %q", got)
	}
}
