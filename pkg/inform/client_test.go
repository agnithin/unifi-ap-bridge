package inform

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestInformPostDoesNotForwardRedirects(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		client := NewInformClient("http://controller/inform", time.Second, "")
		requests := 0
		client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests++
			if requests > 1 {
				return nil, errors.New("redirect leaked to another endpoint")
			}
			return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"http://other/inform"}}, Body: http.NoBody, Request: r}, nil
		})
		_, err := client.Post(context.Background(), make([]byte, HeaderSize+1))
		var response *HTTPError
		if !errors.As(err, &response) || response.StatusCode != status || requests != 1 {
			t.Fatalf("redirect %d: requests=%d err=%v", status, requests, err)
		}
	}
}
