// Package sourcetest supplies shared privacy acceptance fixtures.
package sourcetest

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type guard struct{ calls atomic.Int64 }

func (g *guard) RoundTrip(*http.Request) (*http.Response, error) {
	g.calls.Add(1)
	return nil, fmt.Errorf("unexpected outbound request")
}

// Guard rejects all application HTTP requests and fails if any were attempted.
// Tests using it must not run in parallel because it replaces the default transport.
func Guard(t *testing.T) {
	t.Helper()
	previous := http.DefaultTransport
	g := &guard{}
	http.DefaultTransport = g
	t.Cleanup(func() {
		http.DefaultTransport = previous
		if n := g.calls.Load(); n != 0 {
			t.Errorf("source check attempted %d outbound requests", n)
		}
	})
}

// NuGetConfig creates an official private-source mapping containing a credential sentinel.
func NuGetConfig(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "nuget.config")
	if err := os.WriteFile(file, []byte(`<configuration><packageSources><clear/><add key="internal" value="https://user:credential-sentinel@private.invalid/index.json"/></packageSources><packageSourceMapping><packageSource key="internal"><package pattern="Company.*"/></packageSource></packageSourceMapping></configuration>`), 0600); err != nil {
		t.Fatal(err)
	}
	return file
}
