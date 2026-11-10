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

func TestDartSources(t *testing.T) {
	t.Setenv("PUB_HOSTED_URL", "")
	for _, tt := range []struct {
		config  string
		blocked bool
	}{
		{"dependencies:\n  secret: ^1.0.0\n", false},
		{"dependencies:\n  secret: {hosted: https://private.invalid, version: ^1}\n", true},
		{"dependencies:\n  secret: ^1\ndependency_overrides:\n  secret: {path: ../secret}\n", true},
		{"packages:\n  secret:\n    source: hosted\n    description: {name: secret, url: https://pub.dev}\n", false},
		{"packages:\n  secret:\n    source: git\n    description: {url: https://private.invalid/secret}\n", true},
	} {
		file := filepath.Join(t.TempDir(), "pubspec.yaml")
		if err := os.WriteFile(file, []byte(tt.config), 0600); err != nil {
			t.Fatal(err)
		}
		q := models.Query{Ecosystem: models.EcosystemPub, Package: "secret", SourceFile: file}
		reason, err := sourceRestriction(context.Background(), q)
		if err != nil || (reason != "") != tt.blocked {
			t.Fatalf("%q %v", reason, err)
		}
		if tt.blocked {
			assertUnavailableSource(t, q)
		}
	}
	t.Setenv("PUB_HOSTED_URL", "https://private.invalid")
	assertUnavailableSource(t, models.Query{Ecosystem: models.EcosystemPub, Package: "secret"})
}

func TestJavaScriptManagerSources(t *testing.T) {
	for _, tt := range []struct {
		file, config string
		blocked      bool
	}{
		{".yarnrc.yml", "npmRegistryServer: https://registry.yarnpkg.com\n", false},
		{".yarnrc.yml", "npmScopes:\n  company:\n    npmRegistryServer: https://private.invalid\n", true},
		{"pnpm-workspace.yaml", "registry: https://private.invalid\n", true},
		{"pnpm-workspace.yaml", "registries:\n  '@company': https://private.invalid\n", true},
		{".yarnrc", "--install.registry \"https://private.invalid\"\n", true},
		{".yarnrc", "\"@company:registry\" \"https://private.invalid\"\n", true},
		{"pnpm-workspace.yaml", "registries:\n  'https://private.invalid/':\n    scopes: ['@company']\n", true},
		{".yarnrc", "registry \"https://registry.yarnpkg.com\"\n", false},
		{".yarnrc", "registry \"https://private.invalid\"\n", true},
	} {
		file := filepath.Join(t.TempDir(), tt.file)
		if err := os.WriteFile(file, []byte(tt.config), 0600); err != nil {
			t.Fatal(err)
		}
		q := models.Query{Ecosystem: models.EcosystemNPM, Package: "@company/secret", SourceFile: file}
		reason, err := sourceRestriction(context.Background(), q)
		if err != nil || (reason != "") != tt.blocked {
			t.Fatalf("%q %v", reason, err)
		}
		if tt.blocked {
			assertUnavailableSource(t, q)
		}
	}
}
