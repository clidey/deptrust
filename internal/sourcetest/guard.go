// Package sourcetest supplies shared privacy acceptance fixtures.
package sourcetest

import (
	"fmt"
	"github.com/clidey/deptrust/internal/models"
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

// ArtifactQueries supplies private/unsupported official artifacts for every native reader.
func ArtifactQueries(t *testing.T) []models.Query {
	t.Helper()
	fixtures := []struct {
		ecosystem       models.Ecosystem
		name, pkg, body string
	}{
		{models.EcosystemPackagist, "composer.json", "company/secret", `{"repositories":[{"type":"composer","url":"https://credential-sentinel@private.invalid"}]}`},
		{models.EcosystemPub, "pubspec.yaml", "secret", "dependencies:\n  secret: {hosted: https://private.invalid}\n"},
		{models.EcosystemPyPI, "uv.lock", "secret", "[[package]]\nname='secret'\nsource={registry='https://private.invalid/simple'}\n"},
		{models.EcosystemNPM, ".yarnrc.yml", "@company/secret", "npmRegistryServer: https://private.invalid\n"},
		{models.EcosystemHackage, "plan.json", "secret", `{"install-plan":[{"pkg-name":"secret","pkg-src":{"type":"local","path":"../secret"}}]}`},
		{models.EcosystemCocoaPods, "Podfile.lock", "Secret", "SPEC REPOS:\n  private:\n    - Secret\n"},
		{models.EcosystemHex, "package.json", "secret", `{"name":"secret","repository":"hexpm:company","html_url":"https://hex.pm/packages/secret"}`},
		{models.EcosystemGitHubActions, "repository.json", "company/secret", `{"full_name":"company/secret","private":true,"visibility":"private","html_url":"https://github.com/company/secret"}`},
		{models.EcosystemMaven, "effective-settings.xml", "company:secret", `<settings><mirrors><mirror><url>https://private.invalid/maven</url></mirror></mirrors></settings>`},
	}
	queries := []models.Query{{Ecosystem: models.EcosystemNuGet, Package: "Company.Secret", NuGetConfig: NuGetConfig(t)}}
	for _, fixture := range fixtures {
		file := filepath.Join(t.TempDir(), fixture.name)
		if err := os.WriteFile(file, []byte(fixture.body), 0600); err != nil {
			t.Fatal(err)
		}
		queries = append(queries, models.Query{Ecosystem: fixture.ecosystem, Package: fixture.pkg, SourceFile: file})
	}
	return queries
}
