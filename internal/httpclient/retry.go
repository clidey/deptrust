package httpclient

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxAttempts       = 3
	baseRetryDelay    = 250 * time.Millisecond
	maxRetryAfterWait = 2 * time.Second
)

type retryTransport struct {
	base  http.RoundTripper
	now   func() time.Time
	sleep func(context.Context, time.Duration) error
}

func New() *http.Client {
	return &http.Client{
		Transport: &retryTransport{
			base:  http.DefaultTransport,
			now:   time.Now,
			sleep: sleepContext,
		},
		Timeout: 15 * time.Second,
	}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.base == nil {
		t.base = http.DefaultTransport
	}
	if t.now == nil {
		t.now = time.Now
	}
	if t.sleep == nil {
		t.sleep = sleepContext
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		attemptReq, err := requestForAttempt(req, attempt)
		if err != nil {
			return nil, err
		}
		resp, err := t.base.RoundTrip(attemptReq)
		if !shouldRetry(req, resp, err) || attempt == maxAttempts-1 {
			return resp, err
		}

		delay, retry := t.retryDelay(resp, attempt)
		if !retry {
			return resp, err
		}
		closeResponse(resp)
		if err := t.sleep(req.Context(), delay); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func requestForAttempt(req *http.Request, attempt int) (*http.Request, error) {
	cloned := req.Clone(req.Context())
	if attempt == 0 || req.Body == nil {
		return cloned, nil
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	cloned.Body = body
	return cloned, nil
}

func shouldRetry(req *http.Request, resp *http.Response, err error) bool {
	if req.Context().Err() != nil || !replayable(req) {
		return false
	}
	if err != nil {
		return true
	}
	if resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func replayable(req *http.Request) bool {
	switch req.Method {
	case http.MethodGet, http.MethodHead:
		return true
	case http.MethodPost:
		return req.Body == nil || req.GetBody != nil
	default:
		return false
	}
}

func (t *retryTransport) retryDelay(resp *http.Response, attempt int) (time.Duration, bool) {
	if resp != nil {
		if value := strings.TrimSpace(resp.Header.Get("Retry-After")); value != "" {
			delay, ok := parseRetryAfter(value, t.now())
			if ok && delay > maxRetryAfterWait {
				return 0, false
			}
			if ok {
				return delay, true
			}
		}
	}
	return baseRetryDelay * time.Duration(1<<attempt), true
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := when.Sub(now)
	if delay < 0 {
		delay = 0
	}
	return delay, true
}

func closeResponse(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
