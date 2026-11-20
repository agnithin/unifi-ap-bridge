package telemetry

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMappingRefreshKeepsValidFileOnBadResponses(t *testing.T) {
	valid := `{"guest":"0123456789abcdef01234567"}`
	response := valid
	client := &http.Client{Transport: mappingTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}
	path := filepath.Join(t.TempDir(), "mapping.json")
	refresh := func() error {
		return refreshWLANMapping(context.Background(), client, "http://mapping.test/file", path)
	}
	if err := refresh(); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil || before.Mode().Perm() != 0600 {
		t.Fatalf("mapping mode: %v", err)
	}
	for _, body := range []string{`{"guest":"bad"}`, `{}`, strings.Repeat("x", maxWLANMappingFileBytes+1)} {
		response = body
		if err := refresh(); err == nil {
			t.Fatal("accepted invalid mapping")
		}
		current, err := os.ReadFile(path)
		if err != nil || string(current) != valid {
			t.Fatalf("lost the prior mapping: %v", err)
		}
	}
	response = valid
	if err := refresh(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("unchanged mapping was rewritten: %v", err)
	}
}

type mappingTransport func(*http.Request) (*http.Response, error)

func (f mappingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
