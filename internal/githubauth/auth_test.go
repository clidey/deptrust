package githubauth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func testProvider(values map[string]string) *Provider {
	return &Provider{getenv: func(name string) string { return values[name] }, timeout: 20 * time.Millisecond}
}

func TestProviderPrecedence(t *testing.T) {
	provider := testProvider(map[string]string{
		tokenEnv:       "deptrust-token",
		githubTokenEnv: "github-token",
		ghTokenEnv:     "gh-token",
	})
	if got := provider.Token(); got != "deptrust-token" {
		t.Fatalf("Token() = %q, want preferred token", got)
	}
	provider.getenv = func(name string) string {
		if name == githubTokenEnv {
			return "github-token"
		}
		if name == ghTokenEnv {
			return "gh-token"
		}
		return ""
	}
	if got := provider.Token(); got != "github-token" {
		t.Fatalf("Token() = %q, want GITHUB_TOKEN", got)
	}
}

func TestProviderGHFallbackIsOptIn(t *testing.T) {
	run := func(context.Context) ([]byte, error) { return []byte("gh-token"), nil }
	provider := testProvider(map[string]string{})
	provider.runGH = run
	if got := provider.Token(); got != "" {
		t.Fatalf("Token() = %q without opt-in, want empty", got)
	}
	provider.getenv = func(name string) string {
		if name == githubAuthEnv {
			return "gh"
		}
		return ""
	}
	if got := provider.Token(); got != "gh-token" {
		t.Fatalf("Token() = %q with opt-in, want gh token", got)
	}
}

func TestProviderGHFallbackTimeoutAndErrorsAreIgnored(t *testing.T) {
	provider := testProvider(map[string]string{githubAuthEnv: "gh"})
	called := false
	provider.runGH = func(ctx context.Context) ([]byte, error) {
		called = true
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("gh context has no deadline")
		}
		return nil, errors.New("gh is not authenticated")
	}
	if got := provider.Token(); got != "" || !called {
		t.Fatalf("Token() = %q, called = %t; want no token and one attempted fallback", got, called)
	}
}

func TestTransportAuthorizationAndHostIsolation(t *testing.T) {
	provider := testProvider(map[string]string{tokenEnv: "secret-token"})
	var seen []*http.Request
	transport := NewTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seen = append(seen, req)
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	}), provider)
	for _, target := range []string{"https://api.github.com/advisories", "https://example.com/"} {
		req, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	if got := seen[0].Header.Get("Authorization"); got != "Bearer secret-token" {
		t.Fatalf("GitHub Authorization = %q", got)
	}
	if got := seen[1].Header.Get("Authorization"); got != "" {
		t.Fatalf("non-GitHub Authorization = %q, want empty", got)
	}
}

func TestTransportDoesNotAddAuthorizationWithoutToken(t *testing.T) {
	provider := testProvider(map[string]string{})
	transport := NewTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want empty", got)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	}), provider)
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/advisories", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

func TestTransportRedactsTokenFromErrors(t *testing.T) {
	provider := testProvider(map[string]string{tokenEnv: "secret-token"})
	transport := NewTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport failed with secret-token")
	}), provider)
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/", nil)
	_, err := transport.RoundTrip(req)
	if err == nil || strings.Contains(err.Error(), "secret-token") || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("redacted error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
