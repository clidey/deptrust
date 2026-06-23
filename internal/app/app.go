package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/clidey/deptrust/internal/models"
	"github.com/clidey/deptrust/internal/osv"
	"github.com/clidey/deptrust/internal/registry"
	"github.com/clidey/deptrust/internal/risk"
)

type App struct {
	registry registry.Resolver
	osv      vulnerabilityClient
	now      func() time.Time
}

type vulnerabilityClient interface {
	Query(ctx context.Context, pkg models.PackageVersion) ([]models.Vulnerability, error)
}

func New() App {
	client := &http.Client{Timeout: 15 * time.Second}
	return App{
		registry: registry.New(client),
		osv:      osv.New(client),
		now:      time.Now,
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
	result := a.checkResolved(ctx, resolved)
	if query.Version == "" || strings.EqualFold(query.Version, models.LatestVersion) {
		result.ResolvedFromVersionRequest = models.LatestVersion
	}
	return result, nil
}

func (a App) checkResolved(ctx context.Context, resolved registry.VersionInfo) models.CheckResult {
	pkg := models.PackageVersion{
		Ecosystem:   resolved.Ecosystem,
		Package:     resolved.Package,
		Version:     resolved.Version,
		Latest:      resolved.Latest,
		PublishedAt: resolved.PublishedAt,
	}

	vulns, providerErrors := a.queryVulnerabilities(ctx, pkg)
	sortVulnerabilities(vulns)
	signals := a.signals(pkg)
	assessment := risk.Score(pkg, vulns, signals, providerErrors)

	result := models.CheckResult{
		Ecosystem:                 pkg.Ecosystem,
		Package:                   pkg.Package,
		Version:                   pkg.Version,
		LatestVersion:             pkg.Latest,
		PublishedAt:               pkg.PublishedAt,
		KnownVulnerabilitiesFound: len(vulns) > 0,
		SafeToUse:                 assessment.SafeToUse,
		ShouldInstall:             assessment.SafeToUse,
		RiskScore:                 assessment.RiskScore,
		Classification:            assessment.Classification,
		Recommendation:            assessment.Recommendation,
		Reason:                    decisionReason(assessment.Recommendation, vulns, signals, providerErrors),
		NextAction:                nextAction(assessment.Recommendation, len(vulns), len(signals), len(providerErrors)),
		Summary:                   assessment.Summary,
		Signals:                   signals,
		Vulnerabilities:           vulns,
		ProviderErrors:            providerErrors,
	}
	return result
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
		result.SafeAlternatives = []string{latest.Version}
		result.SuggestedVersionCheck = &latest
		result.Recommendation = risk.RecommendationAllow
		result.Summary = fmt.Sprintf("Use %s %s. No known vulnerabilities were found for the latest version.", latest.Package, latest.Version)
		return result, nil
	}

	resolved, err := a.registry.Resolve(ctx, query)
	if err != nil {
		return models.SuggestResult{}, err
	}
	for _, version := range resolved.Versions {
		if version == "" || version == latest.Version {
			continue
		}
		candidate := a.checkResolved(ctx, registry.VersionInfo{
			Ecosystem:            resolved.Ecosystem,
			Package:              resolved.Package,
			Version:              version,
			Latest:               resolved.Latest,
			Versions:             resolved.Versions,
			PublishedAt:          resolved.PublishedAtByVersion[version],
			PublishedAtByVersion: resolved.PublishedAtByVersion,
		})
		result.CheckedVersions = append(result.CheckedVersions, candidate.Version)
		result.ProviderErrors = appendProviderErrors(result.ProviderErrors, candidate.ProviderErrors)
		if candidate.Recommendation == risk.RecommendationAllow {
			result.SuggestedVersion = candidate.Version
			result.SafeAlternatives = []string{candidate.Version}
			result.SuggestedVersionCheck = &candidate
			result.Recommendation = risk.RecommendationAllow
			result.Summary = fmt.Sprintf("Use %s %s. It is the newest checked version with an allow recommendation.", candidate.Package, candidate.Version)
			return result, nil
		}
	}

	result.Recommendation = risk.RecommendationUnknown
	result.Summary = fmt.Sprintf("No safe version suggestion is available. Checked %d versions and none were classified as allow.", len(result.CheckedVersions))
	return result, nil
}

func (a App) CompareVersions(ctx context.Context, query models.Query, fromVersion, toVersion string) (models.CompareResult, error) {
	fromQuery := query
	fromQuery.Version = strings.TrimSpace(fromVersion)
	toQuery := query
	toQuery.Version = strings.TrimSpace(toVersion)
	if fromQuery.Version == "" || toQuery.Version == "" {
		return models.CompareResult{}, errors.New("compare requires from and to versions")
	}

	from, err := a.CheckPackage(ctx, fromQuery)
	if err != nil {
		return models.CompareResult{}, err
	}
	to, err := a.CheckPackage(ctx, toQuery)
	if err != nil {
		return models.CompareResult{}, err
	}

	resolved, added := diffVulnerabilities(from.Vulnerabilities, to.Vulnerabilities)
	result := models.CompareResult{
		Ecosystem:               from.Ecosystem,
		Package:                 from.Package,
		FromVersion:             from.Version,
		ToVersion:               to.Version,
		ImprovesRisk:            to.RiskScore < from.RiskScore || to.Recommendation == risk.RecommendationAllow && from.Recommendation != risk.RecommendationAllow,
		Recommendation:          compareRecommendation(from, to),
		NextAction:              compareNextAction(to),
		From:                    from,
		To:                      to,
		ResolvedVulnerabilities: resolved,
		AddedVulnerabilities:    added,
	}
	result.Summary = compareSummary(result)
	return result, nil
}

func (a App) queryVulnerabilities(ctx context.Context, pkg models.PackageVersion) ([]models.Vulnerability, []models.ProviderError) {
	vulns, err := a.osv.Query(ctx, pkg)
	if err == nil {
		return dedupeVulnerabilities(vulns), nil
	}
	return nil, []models.ProviderError{{Provider: "OSV", Message: err.Error()}}
}

func (a App) signals(pkg models.PackageVersion) []models.Signal {
	if pkg.PublishedAt == nil {
		return nil
	}
	age := a.now().UTC().Sub(pkg.PublishedAt.UTC())
	if age < 0 {
		age = 0
	}
	if age > 72*time.Hour {
		return nil
	}
	return []models.Signal{
		{
			Type:      "recent_release",
			Severity:  "medium",
			Score:     30,
			Message:   fmt.Sprintf("Version was published recently (%s ago). Review before installing brand-new releases.", humanDuration(age)),
			Source:    "registry",
			CreatedAt: pkg.PublishedAt,
		},
	}
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

func appendProviderErrors(left, right []models.ProviderError) []models.ProviderError {
	if len(right) == 0 {
		return left
	}
	out := append([]models.ProviderError{}, left...)
	seen := map[string]struct{}{}
	for _, item := range out {
		seen[item.Provider+"\x00"+item.Message] = struct{}{}
	}
	for _, item := range right {
		key := item.Provider + "\x00" + item.Message
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, item)
		seen[key] = struct{}{}
	}
	return out
}

func decisionReason(recommendation string, vulns []models.Vulnerability, signals []models.Signal, providerErrors []models.ProviderError) string {
	switch {
	case len(providerErrors) > 0 && len(vulns) == 0:
		return "Could not complete vulnerability lookup."
	case len(vulns) > 0:
		return fmt.Sprintf("Found %d known vulnerability records.", len(vulns))
	case len(signals) > 0:
		return fmt.Sprintf("Found %d non-vulnerability risk signals.", len(signals))
	case recommendation == risk.RecommendationAllow:
		return "No known vulnerabilities or blocking risk signals were found."
	default:
		return "Review recommended by policy."
	}
}

func nextAction(recommendation string, vulnCount, signalCount, providerErrorCount int) string {
	switch recommendation {
	case risk.RecommendationAllow:
		return "install"
	case risk.RecommendationBlock:
		return "do_not_install; use suggest_safe_version or compare_versions to choose a safer version"
	case risk.RecommendationReview:
		if signalCount > 0 && vulnCount == 0 {
			return "review_recent_release_before_installing"
		}
		return "review_advisories_before_installing"
	default:
		if providerErrorCount > 0 {
			return "retry_or_check_manually"
		}
		return "review_before_installing"
	}
}

func diffVulnerabilities(from, to []models.Vulnerability) ([]models.Vulnerability, []models.Vulnerability) {
	fromByID := vulnerabilityMap(from)
	toByID := vulnerabilityMap(to)
	var resolved []models.Vulnerability
	var added []models.Vulnerability
	for id, vuln := range fromByID {
		if _, ok := toByID[id]; !ok {
			resolved = append(resolved, vuln)
		}
	}
	for id, vuln := range toByID {
		if _, ok := fromByID[id]; !ok {
			added = append(added, vuln)
		}
	}
	sortVulnerabilities(resolved)
	sortVulnerabilities(added)
	return resolved, added
}

func vulnerabilityMap(vulns []models.Vulnerability) map[string]models.Vulnerability {
	out := map[string]models.Vulnerability{}
	for _, vuln := range vulns {
		key := vuln.ID
		if key == "" {
			key = vuln.Summary
		}
		out[key] = vuln
	}
	return out
}

func compareRecommendation(from, to models.CheckResult) string {
	if to.Recommendation == risk.RecommendationBlock {
		return risk.RecommendationBlock
	}
	if to.Recommendation == risk.RecommendationUnknown {
		return risk.RecommendationUnknown
	}
	if to.RiskScore < from.RiskScore && to.Recommendation == risk.RecommendationAllow {
		return risk.RecommendationAllow
	}
	return to.Recommendation
}

func compareNextAction(to models.CheckResult) string {
	if to.Recommendation == risk.RecommendationAllow {
		return "upgrade_to_target"
	}
	if to.Recommendation == risk.RecommendationBlock {
		return "do_not_upgrade_to_target"
	}
	return "review_target_before_upgrading"
}

func compareSummary(result models.CompareResult) string {
	if result.ImprovesRisk {
		return fmt.Sprintf("%s %s -> %s improves risk: score %d to %d.", result.Package, result.FromVersion, result.ToVersion, result.From.RiskScore, result.To.RiskScore)
	}
	return fmt.Sprintf("%s %s -> %s does not improve risk: score %d to %d.", result.Package, result.FromVersion, result.ToVersion, result.From.RiskScore, result.To.RiskScore)
}

func humanDuration(duration time.Duration) string {
	if duration < time.Hour {
		minutes := int(duration.Minutes())
		if minutes < 1 {
			minutes = 1
		}
		return fmt.Sprintf("%dm", minutes)
	}
	if duration < 48*time.Hour {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	}
	return fmt.Sprintf("%dd", int(duration.Hours()/24))
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
