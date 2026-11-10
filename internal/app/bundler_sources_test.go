package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestBundlerLockedSources(t *testing.T) {
	if err := exec.Command("ruby", "--disable=rubyopt", "-rbundler", "-e", "exit").Run(); err != nil {
		t.Skip("Ruby and Bundler required")
	}
	for _, tt := range []struct {
		name, body string
		blocked    bool
	}{
		{"public", "GEM\n  remote: https://rubygems.org/\n  specs:\n    example (1.0.0)\n", false},
		{"public alongside path", "GEM\n  remote: https://rubygems.org/\n  specs:\n    example (1.0.0)\n\nPATH\n  remote: ../other\n  specs:\n    other (1.0.0)\n", false},
		{"same name foreign", "GEM\n  remote: https://rubygems.org/\n  specs:\n    example (1.0.0)\n\nGIT\n  remote: https://private.invalid/example\n  specs:\n    example (2.0.0)\n", true},
		{"custom", "GEM\n  remote: https://user:secret@corp.invalid/\n  specs:\n    example (1.0.0)\n", true},
		{"mixed remotes", "GEM\n  remote: https://rubygems.org/\n  remote: https://corp.invalid/\n  specs:\n    example (1.0.0)\n", true},
		{"unlisted", "GEM\n  remote: https://rubygems.org/\n  specs:\n    other (1.0.0)\n", true},
		{"path", "PATH\n  remote: ../example\n  specs:\n    example (1.0.0)\n", true},
		{"git", "GIT\n  remote: https://corp.invalid/example.git\n  revision: deadbeef\n  specs:\n    example (1.0.0)\n", true},
		{"plugin", "PLUGIN\n  remote: https://corp.invalid/plugin\n  specs:\n    example (1.0.0)\n", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			lock := filepath.Join(dir, "Gemfile.lock")
			text := tt.body + "\nPLATFORMS\n  ruby\n\nDEPENDENCIES\n  example\n\nBUNDLED WITH\n   2.5.0\n"
			if err := os.WriteFile(lock, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			// An executable Gemfile must not be evaluated while inspecting the lockfile.
			marker := filepath.Join(dir, "executed")
			if err := os.WriteFile(filepath.Join(dir, "Gemfile"), []byte("File.write("+`"`+marker+`"`+", 'executed')\nraise 'do not evaluate'\n"), 0600); err != nil {
				t.Fatal(err)
			}
			query := models.Query{Ecosystem: models.EcosystemRuby, Package: "example", BundlerLock: lock}
			reason, err := sourceRestriction(context.Background(), query)
			if err != nil || (reason != "") != tt.blocked {
				t.Fatalf("%q %v", reason, err)
			}
			if tt.blocked {
				assertUnavailableSource(t, query)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("Gemfile was evaluated")
			}
			got, err := os.ReadFile(lock)
			if err != nil || string(got) != text {
				t.Fatal("lockfile was modified")
			}
		})
	}
}

func TestBundlerConfigurationErrorsStopLookup(t *testing.T) {
	for _, q := range []models.Query{
		{Ecosystem: models.EcosystemRuby, Package: "secret", BundlerLock: "relative.lock"},
		{Ecosystem: models.EcosystemRuby, Package: "secret", BundlerLock: filepath.Join(t.TempDir(), "missing.lock")},
		{Ecosystem: models.EcosystemNPM, Package: "secret", BundlerLock: "/tmp/lock"},
		{Ecosystem: models.EcosystemRuby, Package: "secret", BundlerLock: "/tmp/lock", ProjectDir: "/tmp/project"},
	} {
		if _, err := (App{}).CheckPackage(context.Background(), q); err == nil {
			t.Fatal("expected error before lookup")
		}
	}
}

func TestNuGetEmptyMappingDisablesMapping(t *testing.T) {
	for _, tt := range []struct {
		mapping string
		blocked bool
	}{
		{"", false}, {"<packageSourceMapping/>", false},
	} {
		file := filepath.Join(t.TempDir(), "nuget.config")
		text := `<configuration><packageSources><clear/><add key="public" value="https://api.nuget.org/v3/index.json"/></packageSources>` + tt.mapping + `</configuration>`
		if err := os.WriteFile(file, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		q := models.Query{Ecosystem: models.EcosystemNuGet, Package: "Secret", NuGetConfig: file}
		reason, err := sourceRestriction(context.Background(), q)
		if err != nil || (reason != "") != tt.blocked {
			t.Fatalf("%q %v", reason, err)
		}
		if tt.blocked {
			assertUnavailableSource(t, q)
		}
	}
}
