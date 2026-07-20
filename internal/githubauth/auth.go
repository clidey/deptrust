package githubauth

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	tokenEnv       = "DEPTRUST_GITHUB_TOKEN"
	githubTokenEnv = "GITHUB_TOKEN"
	ghTokenEnv     = "GH_TOKEN"
	githubAuthEnv  = "DEPTRUST_GITHUB_AUTH"
	ghTimeout      = 2 * time.Second
)

// Provider obtains a GitHub token without persisting it. The function fields
// are kept injectable so credential selection and gh failures can be tested
// without invoking the user's environment.
type Provider struct {
	getenv  func(string) string
	runGH   func(context.Context) ([]byte, error)
	timeout time.Duration
}

func NewProvider() *Provider {
	return &Provider{
		getenv:  os.Getenv,
		runGH:   runGHAuthToken,
		timeout: ghTimeout,
	}
}

// Token returns the first configured credential. The gh fallback is strictly
// opt-in and its output is never included in an error or diagnostic.
func (p *Provider) Token() string {
	if p == nil {
		return ""
	}
	getenv := p.getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	for _, name := range []string{tokenEnv, githubTokenEnv, ghTokenEnv} {
		if token := strings.TrimSpace(getenv(name)); token != "" {
			return token
		}
	}
	if strings.EqualFold(strings.TrimSpace(getenv(githubAuthEnv)), "gh") {
		timeout := p.timeout
		if timeout <= 0 {
			timeout = ghTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if runGH := p.runGH; runGH != nil {
			if output, err := runGH(ctx); err == nil {
				return strings.TrimSpace(string(output))
			}
		}
	}
	return ""
}

func runGHAuthToken(ctx context.Context) ([]byte, error) {
	// Output, rather than CombinedOutput, ensures gh's diagnostics cannot be
	// accidentally copied into DepTrust's error or JSON output.
	return exec.CommandContext(ctx, "gh", "auth", "token").Output()
}

// Redact removes a token from a string without adding any credential-related
// detail. It is intended for errors received from injectable HTTP clients.
func (p *Provider) Redact(value string) string {
	token := p.Token()
	if token == "" {
		return value
	}
	return strings.ReplaceAll(value, token, "[REDACTED]")
}

// Transport adds authentication only to the official HTTPS GitHub API host.
// It is shared by advisory queries and GitHub Actions API registry lookups.
type Transport struct {
	Base     http.RoundTripper
	Provider *Provider
}

func NewTransport(base http.RoundTripper, provider *Provider) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	if provider == nil {
		provider = NewProvider()
	}
	return &Transport{Base: base, Provider: provider}
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	if t.Base == nil {
		t.Base = http.DefaultTransport
	}
	request := req
	provider := t.Provider
	if provider == nil {
		provider = NewProvider()
	}
	if isGitHubAPI(req) {
		if token := provider.Token(); token != "" {
			request = req.Clone(req.Context())
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := t.Base.RoundTrip(request)
	if err != nil {
		return resp, errorsWithoutToken(provider, err)
	}
	return resp, nil
}

func isGitHubAPI(req *http.Request) bool {
	return req != nil && req.URL != nil && strings.EqualFold(req.URL.Scheme, "https") && strings.EqualFold(req.URL.Host, "api.github.com")
}

type redactedError struct{ message string }

func (e redactedError) Error() string { return e.message }

func errorsWithoutToken(provider *Provider, err error) error {
	if err == nil {
		return nil
	}
	return redactedError{message: provider.Redact(err.Error())}
}
