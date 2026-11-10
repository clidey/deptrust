package app

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/clidey/deptrust/internal/models"
	"github.com/clidey/deptrust/internal/sourcetest"
)

func TestMixLockSourcesWithoutEvaluation(t *testing.T) {
	fixtures := t.TempDir()
	sourcetest.Elixir(t, fixtures)
	marker := filepath.Join(fixtures, "executed")
	for _, tt := range []struct {
		name, lock string
		blocked    bool
	}{
		{"public", `%{"secret" => {:hex, :secret, "1.0.0", "hash", [:mix], [], "hexpm", "hash"}}`, false},
		{"private", `%{"secret" => {:hex, :secret, "1.0.0", "hash", [:mix], [], "hexpm:company", "hash"}}`, true},
		{"git", `%{"secret" => {:git, "https://private.invalid/secret", "revision", []}}`, true},
		{"alias", `%{"secret" => {:hex, :different, "1.0.0", "hash", [:mix], [], "hexpm", "hash"}}`, true},
		{"unlisted", `%{"other" => {:hex, :other, "1.0.0", "hash", [:mix], [], "hexpm", "hash"}}`, true},
		{"executable", `%{"secret" => (File.write!(` + strconv.Quote(marker) + `, "executed"); {:hex, :secret, "1.0.0", "hash", [:mix], [], "hexpm", "hash"})}`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := os.MkdirTemp(fixtures, "case-")
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, "mix.lock")
			if err := os.WriteFile(file, []byte(tt.lock), 0600); err != nil {
				t.Fatal(err)
			}
			q := models.Query{Ecosystem: models.EcosystemHex, Package: "secret", SourceFile: file}
			reason, err := sourceRestriction(context.Background(), q)
			if err != nil || (reason != "") != tt.blocked {
				t.Fatalf("%q %v", reason, err)
			}
			if tt.blocked {
				got, err := (App{providers: []vulnerabilityClient{forbiddenPrivateProvider{}}}).CheckPackage(context.Background(), q)
				if err != nil || got.Recommendation != "unknown" {
					t.Fatalf("%+v %v", got, err)
				}
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("lockfile code executed")
			}
			data, err := os.ReadFile(file)
			if err != nil || string(data) != tt.lock {
				t.Fatal("lockfile modified")
			}
		})
	}
}
