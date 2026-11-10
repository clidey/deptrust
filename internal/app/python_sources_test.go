package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestPythonSourceArtifacts(t *testing.T) {
	for _, key := range []string{"PIP_INDEX_URL", "PIP_EXTRA_INDEX_URL", "UV_INDEX_URL", "UV_EXTRA_INDEX_URL", "UV_DEFAULT_INDEX", "UV_INDEX", "PIP_CONFIG_FILE", "PIP_FIND_LINKS", "PIP_NO_INDEX", "UV_FIND_LINKS", "UV_NO_INDEX"} {
		t.Setenv(key, "")
	}
	for _, tt := range []struct {
		file, config string
		blocked      bool
	}{
		{"uv.lock", "[[package]]\nname='secret'\nsource={registry='https://pypi.org/simple'}\n", false},
		{"uv.lock", "[[package]]\nname='secret'\nsource={registry='https://private.invalid/simple'}\n", true},
		{"pyproject.toml", "[project]\ndependencies=['secret']\n[tool.uv.sources]\nsecret={index='internal'}\n[[tool.uv.index]]\nname='internal'\nurl='https://private.invalid/simple'\nexplicit=true\n", true},
		{"pyproject.toml", "[project]\ndependencies=['secret']\n[tool.uv]\n", false},
		{"Pipfile", "[[source]]\nname='private'\nurl='https://private.invalid/simple'\n[packages]\nsecret={version='*',index='private'}\n", true},
		{"Pipfile.lock", `{"_meta":{"sources":[{"name":"pypi","url":"https://pypi.org/simple"}]},"default":{"secret":{"version":"==1"}}}`, false},
		{"Pipfile.lock", `{"default":{"secret":{"version":"==1"}}}`, true},
		{"pip.conf", "[global]\nindex-url=https://private.invalid/simple\n", true},
		{"pip.conf", "[global]\nindex-url=https://private.invalid/simple\n[install]\nindex-url=https://pypi.org/simple\n", false},
	} {
		t.Run(tt.file+tt.config, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), tt.file)
			if err := os.WriteFile(file, []byte(tt.config), 0600); err != nil {
				t.Fatal(err)
			}
			q := models.Query{Ecosystem: models.EcosystemPyPI, Package: "secret", SourceFile: file}
			reason, err := sourceRestriction(context.Background(), q)
			if err != nil || (reason != "") != tt.blocked {
				t.Fatalf("%q %v", reason, err)
			}
			if tt.blocked {
				assertUnavailableSource(t, q)
			}
		})
	}
	t.Setenv("PIP_EXTRA_INDEX_URL", "https://private.invalid/simple")
	assertUnavailableSource(t, models.Query{Ecosystem: models.EcosystemPyPI, Package: "secret"})
}
