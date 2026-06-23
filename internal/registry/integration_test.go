package registry

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/clidey/deptrust/internal/models"
)

func TestIntegrationResolveLiveRegistries(t *testing.T) {
	t.Parallel()

	resolver := New(&http.Client{Timeout: 15 * time.Second})
	tests := []struct {
		name      string
		ecosystem models.Ecosystem
		pkg       string
		version   string
	}{
		{name: "npm", ecosystem: models.EcosystemNPM, pkg: "lodash", version: "4.17.21"},
		{name: "pypi", ecosystem: models.EcosystemPyPI, pkg: "requests", version: "2.32.3"},
		{name: "cargo", ecosystem: models.EcosystemCargo, pkg: "serde", version: "1.0.0"},
		{name: "go", ecosystem: models.EcosystemGo, pkg: "golang.org/x/crypto", version: "v0.31.0"},
		{name: "rubygems", ecosystem: models.EcosystemRuby, pkg: "rails", version: "7.1.0"},
		{name: "nuget", ecosystem: models.EcosystemNuGet, pkg: "Newtonsoft.Json", version: "13.0.3"},
		{name: "maven", ecosystem: models.EcosystemMaven, pkg: "com.google.guava:guava", version: models.LatestVersion},
		{name: "packagist", ecosystem: models.EcosystemPackagist, pkg: "monolog/monolog", version: "3.10.0"},
		{name: "pub", ecosystem: models.EcosystemPub, pkg: "http", version: "1.6.0"},
		{name: "cocoapods", ecosystem: models.EcosystemCocoaPods, pkg: "AFNetworking", version: "4.0.1"},
		{name: "hex", ecosystem: models.EcosystemHex, pkg: "plug", version: models.LatestVersion},
		{name: "hackage", ecosystem: models.EcosystemHackage, pkg: "aeson", version: models.LatestVersion},
		{name: "github-actions", ecosystem: models.EcosystemGitHubActions, pkg: "actions/checkout", version: "v7.0.0"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			got, err := resolver.Resolve(ctx, models.Query{
				Ecosystem: tt.ecosystem,
				Package:   tt.pkg,
				Version:   tt.version,
			})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got.Ecosystem != tt.ecosystem {
				t.Fatalf("Ecosystem = %q, want %q", got.Ecosystem, tt.ecosystem)
			}
			if got.Package != tt.pkg {
				t.Fatalf("Package = %q, want %q", got.Package, tt.pkg)
			}
			if tt.version != models.LatestVersion && got.Version != tt.version {
				t.Fatalf("Version = %q, want %q", got.Version, tt.version)
			}
			if got.Version == "" {
				t.Fatal("Version is empty")
			}
			if got.Latest == "" {
				t.Fatal("Latest is empty")
			}
			if len(got.Versions) == 0 {
				t.Fatal("Versions is empty")
			}
		})
	}
}
