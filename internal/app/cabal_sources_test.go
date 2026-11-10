package app

import (
	"context"
	"github.com/clidey/deptrust/internal/models"
	"os"
	"path/filepath"
	"testing"
)

func TestCabalPlanSources(t *testing.T) {
	for _, tt := range []struct {
		source  string
		blocked bool
	}{
		{`{"type":"repo-tar","repo":{"type":"secure-repo","uri":"https://hackage.haskell.org/"}}`, false},
		{`{"type":"repo-tar","repo":{"type":"secure-repo","uri":"https://private.invalid/"}}`, true},
		{`{"type":"local","path":"../secret"}`, true},
		{`{"type":"source-repo","source-repo":{"location":"https://private.invalid/secret"}}`, true},
		{`{}`, true},
	} {
		file := filepath.Join(t.TempDir(), "plan.json")
		if err := os.WriteFile(file, []byte(`{"install-plan":[{"pkg-name":"secret","pkg-src":`+tt.source+`}]}`), 0600); err != nil {
			t.Fatal(err)
		}
		q := models.Query{Ecosystem: models.EcosystemHackage, Package: "secret", SourceFile: file}
		reason, err := sourceRestriction(context.Background(), q)
		if err != nil || (reason != "") != tt.blocked {
			t.Fatalf("%q %v", reason, err)
		}
		if tt.blocked {
			assertUnavailableSource(t, q)
		}
	}
}
