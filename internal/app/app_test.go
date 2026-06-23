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
}

func (f fakeOSV) Query(_ context.Context, pkg models.PackageVersion) ([]models.Vulnerability, error) {
	if f.vulns == nil {
		return nil, nil
	}
	return f.vulns[pkg.Version], nil
}

func TestSuggestSafeVersionWalksBackFromVulnerableLatest(t *testing.T) {
	service := App{
		registry: fakeRegistry{versions: []string{"3.0.0", "2.0.0", "1.0.0"}},
		osv: fakeOSV{vulns: map[string][]models.Vulnerability{
			"3.0.0": {{ID: "GHSA-new", Severity: "high", Source: "OSV"}},
		}},
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

func TestCheckPackageAddsRecentReleaseSignal(t *testing.T) {
	published := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	service := App{
		registry: fakeRegistry{
			versions:  []string{"1.0.0"},
			published: map[string]*time.Time{"1.0.0": &published},
		},
		osv: fakeOSV{},
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
}

func TestCompareVersionsReportsResolvedVulnerabilities(t *testing.T) {
	service := App{
		registry: fakeRegistry{versions: []string{"2.0.0", "1.0.0"}},
		osv: fakeOSV{vulns: map[string][]models.Vulnerability{
			"1.0.0": {{ID: "GHSA-old", Severity: "high", Source: "OSV"}},
		}},
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
