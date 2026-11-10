package mcp

import (
	"strings"
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestSourceCommandPreservesConfiguration(t *testing.T) {
	for _, q := range []models.Query{
		{ProjectDir: "/tmp/project with ' quotes"},
		{NuGetConfig: "/tmp/project/nuget.config"},
	} {
		original := command("check", "npm", "@corp/secret", "1.2.3")
		got := sourceCommand(original, q)
		if !strings.HasSuffix(got, "--json npm @corp/secret 1.2.3") {
			t.Fatal(got)
		}
		if q.ProjectDir != "" && !strings.Contains(got, "--project-dir "+shellQuote(q.ProjectDir)) {
			t.Fatal(got)
		}
		if q.NuGetConfig != "" && !strings.Contains(got, "--nuget-config "+shellQuote(q.NuGetConfig)) {
			t.Fatal(got)
		}
	}
}
