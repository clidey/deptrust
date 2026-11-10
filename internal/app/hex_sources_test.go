package app

import (
	"context"
	"github.com/clidey/deptrust/internal/models"
	"os"
	"path/filepath"
	"testing"
)

func TestHexRepositoryMetadata(t *testing.T) {
	for _, tt := range []struct {
		body    string
		blocked bool
	}{
		{`{"name":"secret","repository":"hexpm","html_url":"https://hex.pm/packages/secret"}`, false},
		{`{"name":"secret","repository":"hexpm:company","html_url":"https://hex.pm/packages/secret"}`, true},
		{`{"name":"other","repository":"hexpm","html_url":"https://hex.pm/packages/other"}`, true},
		{`{"name":"secret"}`, true},
	} {
		file := filepath.Join(t.TempDir(), "package.json")
		if err := os.WriteFile(file, []byte(tt.body), 0600); err != nil {
			t.Fatal(err)
		}
		q := models.Query{Ecosystem: models.EcosystemHex, Package: "secret", SourceFile: file}
		reason, err := sourceRestriction(context.Background(), q)
		if err != nil || (reason != "") != tt.blocked {
			t.Fatalf("%q %v", reason, err)
		}
		if tt.blocked {
			assertUnavailableSource(t, q)
		}
	}
	assertUnavailableSource(t, models.Query{Ecosystem: models.EcosystemHex, Package: "company/secret"})
}
