package telemetry

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// RefreshWLANMapping validates a bounded download before publishing it.
// A failed refresh leaves the last valid file available after a restart.
func RefreshWLANMapping(ctx context.Context, endpoint, path string) error {
	client := http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{DisableKeepAlives: true},
	}
	return refreshWLANMapping(ctx, &client, endpoint, path)
}

func refreshWLANMapping(ctx context.Context, client *http.Client, endpoint, path string) error {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return errors.New("invalid WLAN mapping URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return errors.New("invalid WLAN mapping request")
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("WLAN mapping download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("WLAN mapping endpoint returned an unexpected status")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxWLANMappingFileBytes+1))
	if err != nil || len(body) > maxWLANMappingFileBytes {
		return errors.New("WLAN mapping download unreadable or oversized")
	}
	if _, ok := parseWLANMapping(body); !ok {
		return errors.New("WLAN mapping download did not pass validation")
	}
	if prior, err := os.Open(path); err == nil {
		old, _ := io.ReadAll(io.LimitReader(prior, maxWLANMappingFileBytes+1))
		info, statErr := prior.Stat()
		_ = prior.Close()
		if statErr == nil && info.Mode().Perm() == 0600 && bytes.Equal(old, body) {
			return nil
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".wlan-mapping-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(body); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp.Name(), path)
}
