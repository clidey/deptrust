package app

import (
	"context"
	"github.com/clidey/deptrust/internal/models"
	"os"
	"path/filepath"
	"testing"
)

func TestCocoaPodsLockedSources(t *testing.T) {
	for _, tt := range []struct {
		data    string
		blocked bool
	}{
		{"SPEC REPOS:\n  trunk:\n    - Secret\n", false},
		{"SPEC REPOS:\n  'https://private.invalid/specs':\n    - Secret\n", true},
		{"SPEC REPOS:\n  trunk:\n    - Secret\nEXTERNAL SOURCES:\n  Secret:\n    :path: ../Secret\n", true},
		{"SPEC REPOS:\n  trunk:\n    - Other\n", true},
	} {
		file := filepath.Join(t.TempDir(), "Podfile.lock")
		if err := os.WriteFile(file, []byte(tt.data), 0600); err != nil {
			t.Fatal(err)
		}
		q := models.Query{Ecosystem: models.EcosystemCocoaPods, Package: "Secret", SourceFile: file}
		reason, err := sourceRestriction(context.Background(), q)
		if err != nil || (reason != "") != tt.blocked {
			t.Fatalf("%q %v", reason, err)
		}
		if tt.blocked {
			assertUnavailableSource(t, q)
		}
	}
}
