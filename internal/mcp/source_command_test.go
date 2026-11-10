package mcp

import (
	"strings"
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestSourceCommandPreservesConfiguration(t *testing.T) {
	for _, q := range []models.Query{
		{SourceFile: "/tmp/source with spaces/uv.lock"},
		{ProjectDir: "/tmp/project with ' quotes"},
		{NuGetConfig: "/tmp/project/nuget.config"},
		{BundlerLock: "/tmp/project with spaces/Gemfile.lock"},
	} {
		original := command("check", "npm", "@corp/secret", "1.2.3")
		got := sourceCommand(original, q)
		if !strings.HasSuffix(got, "--json npm @corp/secret 1.2.3") {
			t.Fatal(got)
		}
		if q.SourceFile != "" && !strings.Contains(got, "--source-file "+shellQuote(q.SourceFile)) {
			t.Fatal(got)
		}
		if q.ProjectDir != "" && !strings.Contains(got, "--project-dir "+shellQuote(q.ProjectDir)) {
			t.Fatal(got)
		}
		if q.BundlerLock != "" && !strings.Contains(got, "--bundler-lock "+shellQuote(q.BundlerLock)) {
			t.Fatal(got)
		}
		if q.NuGetConfig != "" && !strings.Contains(got, "--nuget-config "+shellQuote(q.NuGetConfig)) {
			t.Fatal(got)
		}
	}
}
