package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"deptrust/internal/models"
	"deptrust/internal/osv"
	"deptrust/internal/registry"
	"deptrust/internal/risk"
)

type App struct {
	registry registry.Resolver
	osv      osv.Client
}

func New() App {
	client := &http.Client{Timeout: 15 * time.Second}
	return App{
		registry: registry.New(client),
		osv:      osv.New(client),
	}
}

func (a App) CheckPackage(ctx context.Context, query models.Query) (models.CheckResult, error) {
	if err := query.Validate(); err != nil {
		return models.CheckResult{}, err
	}
	query.Package = strings.TrimSpace(query.Package)
	query.Version = strings.TrimSpace(query.Version)
	if query.Version == "" {
		query.Version = models.LatestVersion
	}

	resolved, err := a.registry.Resolve(ctx, query)
	if err != nil {
		return models.CheckResult{}, err
	}

	pkg := models.PackageVersion{
		Ecosystem: resolved.Ecosystem,
		Package:   resolved.Package,
		Version:   resolved.Version,
		Latest:    resolved.Latest,
	}

	vulns, providerErrors := a.queryVulnerabilities(ctx, pkg)
	sortVulnerabilities(vulns)
	assessment := risk.Score(pkg, vulns, providerErrors)

	result := models.CheckResult{
		Ecosystem:                 pkg.Ecosystem,
		Package:                   pkg.Package,
		Version:                   pkg.Version,
		LatestVersion:             pkg.Latest,
		KnownVulnerabilitiesFound: len(vulns) > 0,
		SafeToUse:                 assessment.SafeToUse,
		RiskScore:                 assessment.RiskScore,
		Classification:            assessment.Classification,
		Recommendation:            assessment.Recommendation,
		Summary:                   assessment.Summary,
		Vulnerabilities:           vulns,
		ProviderErrors:            providerErrors,
	}
	if query.Version == "" || strings.EqualFold(query.Version, models.LatestVersion) {
		result.ResolvedFromVersionRequest = models.LatestVersion
	}
	return result, nil
}

func (a App) SuggestSafeVersion(ctx context.Context, query models.Query) (models.SuggestResult, error) {
	query.Version = models.LatestVersion
	latest, err := a.CheckPackage(ctx, query)
	if err != nil {
		return models.SuggestResult{}, err
	}

	result := models.SuggestResult{
		Ecosystem:           latest.Ecosystem,
		Package:             latest.Package,
		LatestVersion:       latest.Version,
		LatestVersionResult: &latest,
		CheckedVersions:     []string{latest.Version},
		ProviderErrors:      latest.ProviderErrors,
	}

	if latest.Recommendation == risk.RecommendationAllow {
		result.SuggestedVersion = latest.Version
		result.SuggestedVersionCheck = &latest
		result.Recommendation = risk.RecommendationAllow
		result.Summary = fmt.Sprintf("Use %s %s. No known vulnerabilities were found for the latest version.", latest.Package, latest.Version)
		return result, nil
	}

	result.Recommendation = risk.RecommendationUnknown
	result.Summary = fmt.Sprintf("No safe version suggestion is available because the latest version %s was not classified as allow.", latest.Version)
	return result, nil
}

func (a App) queryVulnerabilities(ctx context.Context, pkg models.PackageVersion) ([]models.Vulnerability, []models.ProviderError) {
	vulns, err := a.osv.Query(ctx, pkg)
	if err == nil {
		return dedupeVulnerabilities(vulns), nil
	}
	return nil, []models.ProviderError{{Provider: "OSV", Message: err.Error()}}
}

func ParseQuery(ecosystem, packageName, version string) (models.Query, error) {
	normalized, err := models.NormalizeEcosystem(ecosystem)
	if err != nil {
		return models.Query{}, err
	}
	return models.Query{
		Ecosystem: normalized,
		Package:   strings.TrimSpace(packageName),
		Version:   strings.TrimSpace(version),
	}, nil
}

func IsVersionNotFound(err error) (registry.VersionNotFoundError, bool) {
	var notFound registry.VersionNotFoundError
	if errors.As(err, &notFound) {
		return notFound, true
	}
	return registry.VersionNotFoundError{}, false
}

func dedupeVulnerabilities(vulns []models.Vulnerability) []models.Vulnerability {
	seen := map[string]struct{}{}
	out := make([]models.Vulnerability, 0, len(vulns))
	for _, vuln := range vulns {
		key := vuln.ID
		if key == "" {
			key = vuln.Summary
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, vuln)
	}
	return out
}

func sortVulnerabilities(vulns []models.Vulnerability) {
	sort.SliceStable(vulns, func(i, j int) bool {
		left := risk.SeverityRank(vulns[i].Severity)
		right := risk.SeverityRank(vulns[j].Severity)
		if left != right {
			return left > right
		}
		return vulns[i].ID < vulns[j].ID
	})
}
