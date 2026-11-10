package app

import (
	"context"
	"testing"

	"github.com/clidey/deptrust/internal/models"
	"github.com/clidey/deptrust/internal/risk"
)

func TestPrivateGoSkipsAllExternalLookups(t *testing.T) {
	t.Setenv("GOPRIVATE", "github.com/acme/*")
	// A nil registry and a panicking provider catch any attempted external lookup.
	service := App{providers: []vulnerabilityClient{forbiddenPrivateProvider{}}}
	query := models.Query{Ecosystem: models.EcosystemGo, Package: "github.com/acme/secret", Version: "v1.2.3"}
	for _, version := range []string{"v1.2.3", "latest", ""} {
		query.Version = version
		got, err := service.CheckPackage(context.Background(), query)
		if err != nil {
			t.Fatal(err)
		}
		if got.Recommendation != risk.RecommendationUnknown || got.SafeToUse || got.ShouldInstall || got.AdvisoryCoverage != "none" || got.RegistryVerification != "unverified" {
			t.Fatalf("unexpected private result: %+v", got)
		}
	}
	suggestion, err := service.SuggestSafeVersion(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if suggestion.Recommendation != risk.RecommendationUnknown || suggestion.SuggestedVersion != "" || suggestion.LatestVersion != "" || len(suggestion.CheckedVersions) != 0 {
		t.Fatalf("unexpected suggestion: %+v", suggestion)
	}
	comparison, err := service.CompareVersions(context.Background(), query, "v1.0.0", "v2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Recommendation != risk.RecommendationUnknown || comparison.ImprovesRisk {
		t.Fatalf("unexpected comparison: %+v", comparison)
	}
}

func TestPrivateGoPrefixMatching(t *testing.T) {
	for _, tt := range []struct {
		patterns, module string
		want             bool
	}{
		{"github.com/acme", "github.com/acme/secret/v2", true},
		{"github.com/acme", "github.com/acme-other/secret", false},
		{"*.corp.example.com", "git.corp.example.com/team/module", true},
		{",github.com/acme/,example.com/private", "github.com/acme/secret", true},
		{"github.com/acme/*", "github.com/acme", false},
		{"[", "github.com/acme/secret", false},
		{"", "github.com/acme/secret", false},
	} {
		t.Run(tt.patterns+tt.module, func(t *testing.T) {
			t.Setenv("GOPRIVATE", tt.patterns)
			q := models.Query{Ecosystem: models.EcosystemGo, Package: tt.module}
			if got := isPrivateGoModule(q); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			q.Ecosystem = models.EcosystemNPM
			if isPrivateGoModule(q) {
				t.Fatal("privacy handling applied to npm")
			}
		})
	}
}

type forbiddenPrivateProvider struct{}

func (forbiddenPrivateProvider) Name() string { return "forbidden" }
func (forbiddenPrivateProvider) Query(context.Context, models.PackageVersion) ([]models.Vulnerability, error) {
	panic("private module sent to advisory provider")
}
