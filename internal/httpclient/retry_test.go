package httpclient

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRetryTransportRetriesRateLimit(t *testing.T) {
	attempts := 0
	var delays []time.Duration
	transport := &retryTransport{
		base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			attempts++
			status := http.StatusTooManyRequests
			if attempts == 3 {
				status = http.StatusOK
			}
			return response(status, ""), nil
		}),
		now: time.Now,
		sleep: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
	}

	resp, err := transport.RoundTrip(newRequest(t, http.MethodGet, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if attempts != 3 || resp.StatusCode != http.StatusOK {
		t.Fatalf("attempts/status = %d/%d, want 3/200", attempts, resp.StatusCode)
	}
	if len(delays) != 2 || delays[0] != 250*time.Millisecond || delays[1] != 500*time.Millisecond {
		t.Fatalf("delays = %#v, want bounded exponential delays", delays)
	}
}

func TestRetryTransportHonorsShortRetryAfter(t *testing.T) {
	attempts := 0
	var delay time.Duration
	transport := &retryTransport{
		base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return response(http.StatusTooManyRequests, "1"), nil
			}
			return response(http.StatusOK, ""), nil
		}),
		now: time.Now,
		sleep: func(_ context.Context, value time.Duration) error {
			delay = value
			return nil
		},
	}

	resp, err := transport.RoundTrip(newRequest(t, http.MethodGet, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if attempts != 2 || delay != time.Second {
		t.Fatalf("attempts/delay = %d/%s, want 2/1s", attempts, delay)
	}
}

func TestRetryTransportDoesNotRetryLongRetryAfter(t *testing.T) {
	attempts := 0
	transport := &retryTransport{
		base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			attempts++
			return response(http.StatusTooManyRequests, "60"), nil
		}),
		now:   time.Now,
		sleep: func(context.Context, time.Duration) error { return nil },
	}

	resp, err := transport.RoundTrip(newRequest(t, http.MethodGet, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if attempts != 1 || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("attempts/status = %d/%d, want 1/429", attempts, resp.StatusCode)
	}
}

func TestRetryTransportReplaysPostBody(t *testing.T) {
	attempts := 0
	transport := &retryTransport{
		base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts++
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != `{"query":"value"}` {
				t.Fatalf("body = %q", body)
			}
			if attempts == 1 {
				return response(http.StatusServiceUnavailable, ""), nil
			}
			return response(http.StatusOK, ""), nil
		}),
		now:   time.Now,
		sleep: func(context.Context, time.Duration) error { return nil },
	}

	req := newRequest(t, http.MethodPost, bytes.NewBufferString(`{"query":"value"}`))
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func newRequest(t *testing.T, method string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, "https://example.com/test", body)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func response(status int, retryAfter string) *http.Response {
	header := http.Header{}
	if retryAfter != "" {
		header.Set("Retry-After", retryAfter)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader("response")),
	}
}
