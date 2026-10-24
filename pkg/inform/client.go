package inform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

var (
	ErrHTTPStatus = errors.New("controller returned non-200 status")
)

type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("controller returned HTTP %d", e.StatusCode)
}

type InformClient struct {
	URL        string
	Timeout    time.Duration
	SourceIP   string
	httpClient *http.Client
}

func NewInformClient(url string, timeout time.Duration, sourceIP string) *InformClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	transport := &http.Transport{
		Proxy:                 nil, // Never use HTTP proxies for local inform
		DisableKeepAlives:     false,
		ResponseHeaderTimeout: timeout,
	}

	if sourceIP != "" {
		srcAddr := &net.TCPAddr{IP: net.ParseIP(sourceIP), Port: 0}
		dialer := &net.Dialer{
			LocalAddr: srcAddr,
			Timeout:   timeout,
		}
		transport.DialContext = dialer.DialContext
	}

	return &InformClient{
		URL:      url,
		Timeout:  timeout,
		SourceIP: sourceIP,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}
}

func (c *InformClient) Post(ctx context.Context, packet []byte) ([]byte, error) {
	if len(packet) <= HeaderSize || len(packet) > MaxPacketSize {
		return nil, ErrInvalidPacketSize
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(packet))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-binary")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("inform http transport error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode}
	}

	limited := io.LimitReader(resp.Body, MaxPacketSize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read inform response: %w", err)
	}
	if len(body) > MaxPacketSize {
		return nil, ErrInvalidPacketSize
	}
	return body, nil
}
