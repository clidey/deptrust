package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestComposerRepositories(t *testing.T) {
	for _, tt := range []struct {
		config  string
		blocked bool
	}{
		{`{}`, false},
		{`{"repositories":[{"type":"composer","url":"https://user:secret@private.invalid"}]}`, true},
		{`{"repositories":[{"type":"composer","url":"https://private.invalid","canonical":false}]}`, true},
		{`{"repositories":[{"type":"composer","url":"https://private.invalid","only":["other/*"]}]}`, false},
		{`{"repositories":[{"type":"composer","url":"https://private.invalid","exclude":["company/*"]}]}`, false},
		{`{"repositories":[{"packagist.org":false}]}`, true},
		{`{"repositories":{"packagist.org":false}}`, true},
	} {
		file := filepath.Join(t.TempDir(), "composer.json")
		if err := os.WriteFile(file, []byte(tt.config), 0600); err != nil {
			t.Fatal(err)
		}
		query := models.Query{Ecosystem: models.EcosystemPackagist, Package: "company/secret", SourceFile: file}
		reason, err := sourceRestriction(context.Background(), query)
		if err != nil || (reason != "") != tt.blocked {
			t.Fatalf("%q: %q %v", tt.config, reason, err)
		}
		if tt.blocked {
			assertUnavailableSource(t, query)
		}
	}
}
