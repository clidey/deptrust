package app

import (
	"context"
	"testing"
	"time"

	"github.com/clidey/deptrust/internal/models"
	"github.com/clidey/deptrust/internal/registry"
	"github.com/clidey/deptrust/internal/risk"
)

type fakeRegistry struct {
	versions  []string
	published map[string]*time.Time
}

func (f fakeRegistry) Resolve(_ context.Context, query models.Query) (registry.VersionInfo, error) {
	version := query.Version
	if version == "" || version == models.LatestVersion {
		version = f.versions[0]
	}
	found := false
	for _, item := range f.versions {
		if item == version {
			found = true
			break
		}
	}
	if !found {
		return registry.VersionInfo{}, registry.VersionNotFoundError{Package: query.Package, Version: version, Latest: f.versions[0]}
	}
	return registry.VersionInfo{
		Ecosystem:            query.Ecosystem,
		Package:              query.Package,
		Version:              version,
		Latest:               f.versions[0],
		Versions:             f.versions,
		PublishedAt:          f.published[version],
		PublishedAtByVersion: f.published,
	}, nil
}

type fakeOSV struct {
	vulns map[string][]models.Vulnerability
	err   error
	name  string
}

func (f fakeOSV) Name() string {
	if f.name != "" {
		return f.name
	}
	return "fake"
}

func (f fakeOSV) Query(_ context.Context, pkg models.PackageVersion) ([]models.Vulnerability, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.vulns == nil {
		return nil, nil
	}
	return f.vulns[pkg.Version], nil
}

type fakeAwareProvider struct {
	fakeOSV
	supported bool
}

func (f fakeAwareProvider) Supports(models.Ecosystem) bool {
	return f.supported
}

func TestSuggestSafeVersionWalksBackFromVulnerableLatest(t *testing.T) {
	service := App{
		registry: fakeRegistry{versions: []string{"3.0.0", "2.0.0", "1.0.0"}},
		providers: []vulnerabilityClient{
			fakeOSV{vulns: map[string][]models.Vulnerability{
				"3.0.0": {{ID: "GHSA-new", Severity: "high", Source: "OSV"}},
			}},
		},
		now: time.Now,
	}

	result, err := service.SuggestSafeVersion(context.Background(), models.Query{Ecosystem: models.EcosystemNPM, Package: "pkg"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SuggestedVersion != "2.0.0" {
		t.Fatalf("SuggestedVersion = %q, want 2.0.0", result.SuggestedVersion)
	}
	if result.Recommendation != risk.RecommendationAllow {
		t.Fatalf("Recommendation = %q, want allow", result.Recommendation)
	}
}

func TestSuggestSafeVersionPrefersFixedVersions(t *testing.T) {
	service := App{
		registry: fakeRegistry{versions: []string{"3.0.0", "2.5.0", "2.0.0", "1.0.0"}},
		providers: []vulnerabilityClient{
			fakeOSV{vulns: map[string][]models.Vulnerability{
				"3.0.0": {{
					ID:            "GHSA-new",
					Severity:      "high",
					Source:        "OSV",
					FixedVersions: []string{"2.5.0"},
				}},
				"2.5.0": nil,
			}},
		},
		now: time.Now,
	}

	result, err := service.SuggestSafeVersion(context.Background(), models.Query{Ecosystem: models.EcosystemNPM, Package: "pkg"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SuggestedVersion != "2.5.0" {
		t.Fatalf("SuggestedVersion = %q, want fixed version 2.5.0", result.SuggestedVersion)
	}
	if len(result.CheckedVersions) != 2 {
		t.Fatalf("CheckedVersions = %#v, want latest and fixed version only", result.CheckedVersions)
	}
}

func TestCheckPackageAddsRecentReleaseSignal(t *testing.T) {
	published := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	service := App{
		registry: fakeRegistry{
			versions:  []string{"1.0.0"},
			published: map[string]*time.Time{"1.0.0": &published},
		},
		providers: []vulnerabilityClient{fakeOSV{}},
		now: func() time.Time {
			return published.Add(24 * time.Hour)
		},
	}

	result, err := service.CheckPackage(context.Background(), models.Query{Ecosystem: models.EcosystemNPM, Package: "pkg", Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Recommendation != risk.RecommendationReview {
		t.Fatalf("Recommendation = %q, want review", result.Recommendation)
	}
	if len(result.Signals) != 1 || result.Signals[0].Type != "recent_release" {
		t.Fatalf("Signals = %#v, want recent_release", result.Signals)
	}
	if result.NextAction != "review_recent_release_before_installing" {
		t.Fatalf("NextAction = %q, want review_recent_release_before_installing", result.NextAction)
	}
}

func TestCheckPackageAddsGitHubActionsMutableTagSignal(t *testing.T) {
	service := App{
		registry:  fakeRegistry{versions: []string{"v4", "v4.2.2"}},
		providers: []vulnerabilityClient{fakeOSV{}},
		now:       time.Now,
	}

	result, err := service.CheckPackage(context.Background(), models.Query{Ecosystem: models.EcosystemGitHubActions, Package: "actions/checkout", Version: "v4"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Recommendation != risk.RecommendationReview {
		t.Fatalf("Recommendation = %q, want review", result.Recommendation)
	}
	if len(result.Signals) != 1 || result.Signals[0].Type != "mutable_action_tag" {
		t.Fatalf("Signals = %#v, want mutable_action_tag", result.Signals)
	}
	if result.NextAction != "review_risk_signals_before_installing" {
		t.Fatalf("NextAction = %q, want review_risk_signals_before_installing", result.NextAction)
	}
}

func TestCompareVersionsReportsResolvedVulnerabilities(t *testing.T) {
	service := App{
		registry: fakeRegistry{versions: []string{"2.0.0", "1.0.0"}},
		providers: []vulnerabilityClient{
			fakeOSV{vulns: map[string][]models.Vulnerability{
				"1.0.0": {{ID: "GHSA-old", Severity: "high", Source: "OSV"}},
			}},
		},
		now: time.Now,
	}

	result, err := service.CompareVersions(context.Background(), models.Query{Ecosystem: models.EcosystemNPM, Package: "pkg"}, "1.0.0", "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !result.ImprovesRisk {
		t.Fatal("ImprovesRisk = false, want true")
	}
	if len(result.ResolvedVulnerabilities) != 1 {
		t.Fatalf("ResolvedVulnerabilities = %#v, want one", result.ResolvedVulnerabilities)
	}
}

func TestCompareVersionsRequiresBothVersions(t *testing.T) {
	service := App{}
	_, err := service.CompareVersions(context.Background(), models.Query{Ecosystem: models.EcosystemNPM, Package: "pkg"}, "", "2.0.0")
	if err == nil || err.Error() != "compare requires from and to versions" {
		t.Fatal("expected compare version error")
	}
}

func TestCheckPackageQueriesProvidersAndDedupes(t *testing.T) {
	service := App{
		registry: fakeRegistry{versions: []string{"1.0.0"}},
		providers: []vulnerabilityClient{
			fakeOSV{vulns: map[string][]models.Vulnerability{
				"1.0.0": {{ID: "OSV-1", GHSAIDs: []string{"GHSA-same"}, Severity: "high", Source: "OSV"}},
			}, name: "OSV"},
			fakeOSV{vulns: map[string][]models.Vulnerability{
				"1.0.0": {{ID: "GHSA-same", GHSAIDs: []string{"GHSA-same"}, Severity: "high", Source: "GitHub Advisory DB"}},
			}, name: "GitHub Advisory DB"},
		},
		now: time.Now,
	}

	result, err := service.CheckPackage(context.Background(), models.Query{Ecosystem: models.EcosystemNPM, Package: "pkg", Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Vulnerabilities) != 1 {
		t.Fatalf("Vulnerabilities = %#v, want one deduped advisory", result.Vulnerabilities)
	}
	if len(result.CheckedProviders) != 2 {
		t.Fatalf("CheckedProviders = %#v, want two checked providers", result.CheckedProviders)
	}
	if result.AdvisoryCoverage != "full" {
		t.Fatalf("AdvisoryCoverage = %q, want full", result.AdvisoryCoverage)
	}
}

func TestCheckPackageReportsUnsupportedProviderCoverage(t *testing.T) {
	service := App{
		registry:  fakeRegistry{versions: []string{"1.0.0"}},
		providers: []vulnerabilityClient{fakeAwareProvider{supported: false}},
		now:       time.Now,
	}

	result, err := service.CheckPackage(context.Background(), models.Query{Ecosystem: models.EcosystemNPM, Package: "pkg", Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Recommendation != risk.RecommendationUnknown {
		t.Fatalf("Recommendation = %q, want unknown", result.Recommendation)
	}
	if len(result.ProviderErrors) != 1 {
		t.Fatalf("ProviderErrors = %#v, want unsupported coverage error", result.ProviderErrors)
	}
	if result.AdvisoryCoverage != "none" {
		t.Fatalf("AdvisoryCoverage = %q, want none", result.AdvisoryCoverage)
	}
	if len(result.SkippedProviders) != 1 {
		t.Fatalf("SkippedProviders = %#v, want one skipped provider", result.SkippedProviders)
	}
}
