package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/clidey/deptrust/internal/models"
	"github.com/clidey/deptrust/internal/sourcetest"
)

func TestNullSourceJSONStopsBeforeLookup(t *testing.T) {
	sourcetest.Guard(t)
	file := filepath.Join(t.TempDir(), "composer.json")
	if err := os.WriteFile(file, []byte("null"), 0600); err != nil {
		t.Fatal(err)
	}
	q := models.Query{Ecosystem: models.EcosystemPackagist, Package: "company/secret", SourceFile: file}
	if _, err := sourceRestriction(context.Background(), q); err == nil {
		t.Fatal("null source document accepted as public")
	}
	if _, err := (App{}).CheckPackage(context.Background(), q); err == nil {
		t.Fatal("invalid source context did not stop lookup")
	}
}
