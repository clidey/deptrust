package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/clidey/deptrust/internal/github"
	"github.com/clidey/deptrust/internal/githubauth"
	"github.com/clidey/deptrust/internal/httpclient"
	"github.com/clidey/deptrust/internal/models"
	"github.com/clidey/deptrust/internal/osv"
	"github.com/clidey/deptrust/internal/registry"
	"github.com/clidey/deptrust/internal/risk"
)

type App struct {
	registry  registry.Resolver
	providers []vulnerabilityClient
	now       func() time.Time
}

type vulnerabilityClient interface {
	Name() string
	Query(ctx context.Context, pkg models.PackageVersion) ([]models.Vulnerability, error)
}

type ecosystemAwareProvider interface {
	Supports(ecosystem models.Ecosystem) bool
}

type vulnerabilityQueryResult struct {
	Vulnerabilities  []models.Vulnerability
	ProviderErrors   []models.ProviderError
	CheckedProviders []string
	SkippedProviders []models.SkippedProvider
	AdvisoryCoverage string
	CoverageReason   string
}

func New() App {
	provider := githubauth.NewProvider()
	client := httpclient.NewWithProvider(provider)
	return App{
		registry: registry.New(client),
		providers: []vulnerabilityClient{
			osv.New(client),
			github.NewWithProvider(client, provider),
		},
		now: time.Now,
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
		if strings.EqualFold(query.Version, models.LatestVersion) || definitiveRegistryError(err) || ctx.Err() != nil {
			return models.CheckResult{}, err
		}
		result := a.checkResolved(ctx, registry.VersionInfo{
			Ecosystem: query.Ecosystem,
			Package:   query.Package,
			Version:   query.Version,
		})
		return markRegistryUnverified(result, err), nil
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

	vulnResult := a.queryVulnerabilities(ctx, pkg)
	vulns := vulnResult.Vulnerabilities
	providerErrors := vulnResult.ProviderErrors
	sortVulnerabilities(vulns)
	signals := append([]models.Signal{}, resolved.Signals...)
	signals = append(signals, a.signals(pkg)...)
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
		NextAction:                nextAction(assessment.Recommendation, len(vulns), signals, providerErrors),
		Summary:                   assessment.Summary,
		Signals:                   signals,
		Vulnerabilities:           vulns,
		ProviderErrors:            providerErrors,
		CheckedProviders:          vulnResult.CheckedProviders,
		SkippedProviders:          vulnResult.SkippedProviders,
		AdvisoryCoverage:          vulnResult.AdvisoryCoverage,
		AdvisoryCoverageReason:    vulnResult.CoverageReason,
		RegistryVerification:      "verified",
	}
	return result
}

func definitiveRegistryError(err error) bool {
	var packageNotFound registry.PackageNotFoundError
	if errors.As(err, &packageNotFound) {
		return true
	}
	var versionNotFound registry.VersionNotFoundError
	return errors.As(err, &versionNotFound)
}

func markRegistryUnverified(result models.CheckResult, registryErr error) models.CheckResult {
	result.RegistryVerification = "unverified"
	result.RegistryVerificationReason = registryErr.Error()
	result.Summary += " Registry verification was unavailable, so package existence and release metadata were not confirmed."
	if result.Recommendation != risk.RecommendationAllow {
		return result
	}
	result.SafeToUse = false
	result.ShouldInstall = false
	result.Recommendation = risk.RecommendationUnknown
	if len(result.Vulnerabilities) == 0 && len(result.Signals) == 0 {
		result.Classification = risk.ClassificationUnknown
	}
	result.Reason = "Registry verification was unavailable; advisory results alone cannot establish that this exact package version is valid or safe."
	result.NextAction = "retry_or_check_manually"
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
		CheckedProviders:    latest.CheckedProviders,
		SkippedProviders:    latest.SkippedProviders,
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
	for _, version := range preferredFixedVersions(latest.Vulnerabilities, resolved.Versions) {
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
		result.CheckedVersions = appendUniqueString(result.CheckedVersions, candidate.Version)
		result.ProviderErrors = appendProviderErrors(result.ProviderErrors, candidate.ProviderErrors)
		result.CheckedProviders = appendUniqueStrings(result.CheckedProviders, candidate.CheckedProviders...)
		result.SkippedProviders = appendSkippedProviders(result.SkippedProviders, candidate.SkippedProviders...)
		if candidate.Recommendation == risk.RecommendationAllow {
			result.SuggestedVersion = candidate.Version
			result.SafeAlternatives = []string{candidate.Version}
			result.SuggestedVersionCheck = &candidate
			result.Recommendation = risk.RecommendationAllow
			result.Summary = fmt.Sprintf("Use %s %s. It is a provider-reported fixed version with an allow recommendation.", candidate.Package, candidate.Version)
			return result, nil
		}
	}
	for _, version := range resolved.Versions {
		if version == "" || version == latest.Version || containsString(result.CheckedVersions, version) {
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
		result.CheckedVersions = appendUniqueString(result.CheckedVersions, candidate.Version)
		result.ProviderErrors = appendProviderErrors(result.ProviderErrors, candidate.ProviderErrors)
		result.CheckedProviders = appendUniqueStrings(result.CheckedProviders, candidate.CheckedProviders...)
		result.SkippedProviders = appendSkippedProviders(result.SkippedProviders, candidate.SkippedProviders...)
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

func (a App) queryVulnerabilities(ctx context.Context, pkg models.PackageVersion) vulnerabilityQueryResult {
	if len(a.providers) == 0 {
		return vulnerabilityQueryResult{
			Vulnerabilities: []models.Vulnerability{},
			ProviderErrors: []models.ProviderError{{
				Provider: "deptrust",
				Message:  "no vulnerability providers configured",
			}},
			AdvisoryCoverage: "none",
			CoverageReason:   "no vulnerability providers configured",
		}
	}

	providers := make([]vulnerabilityClient, 0, len(a.providers))
	var skippedProviders []models.SkippedProvider
	for _, provider := range a.providers {
		if aware, ok := provider.(ecosystemAwareProvider); ok && !aware.Supports(pkg.Ecosystem) {
			skippedProviders = append(skippedProviders, models.SkippedProvider{
				Provider: provider.Name(),
				Reason:   fmt.Sprintf("unsupported ecosystem %s", pkg.Ecosystem),
			})
			continue
		}
		providers = append(providers, provider)
	}
	if len(providers) == 0 {
		return vulnerabilityQueryResult{
			Vulnerabilities: []models.Vulnerability{},
			ProviderErrors: []models.ProviderError{{
				Provider: "deptrust",
				Message:  fmt.Sprintf("no vulnerability provider supports ecosystem %s", pkg.Ecosystem),
			}},
			SkippedProviders: skippedProviders,
			AdvisoryCoverage: "none",
			CoverageReason:   "no vulnerability provider supports this ecosystem",
		}
	}

	type providerResult struct {
		provider string
		vulns    []models.Vulnerability
		err      error
	}

	results := make(chan providerResult, len(providers))
	var wg sync.WaitGroup
	for _, provider := range providers {
		wg.Add(1)
		go func(provider vulnerabilityClient) {
			defer wg.Done()
			vulns, err := provider.Query(ctx, pkg)
			results <- providerResult{provider: provider.Name(), vulns: vulns, err: err}
		}(provider)
	}
	wg.Wait()
	close(results)

	var vulns []models.Vulnerability
	var providerErrors []models.ProviderError
	var checkedProviders []string
	for result := range results {
		checkedProviders = appendUniqueString(checkedProviders, result.provider)
		if result.err != nil {
			providerErrors = append(providerErrors, models.ProviderError{Provider: result.provider, Message: result.err.Error()})
			continue
		}
		vulns = append(vulns, result.vulns...)
	}
	vulns = dedupeVulnerabilities(vulns)
	coverage, reason := advisoryCoverage(checkedProviders, skippedProviders, providerErrors)
	return vulnerabilityQueryResult{
		Vulnerabilities:  vulns,
		ProviderErrors:   providerErrors,
		CheckedProviders: checkedProviders,
		SkippedProviders: skippedProviders,
		AdvisoryCoverage: coverage,
		CoverageReason:   reason,
	}
}

func (a App) signals(pkg models.PackageVersion) []models.Signal {
	var signals []models.Signal
	if pkg.PublishedAt == nil {
		signals = append(signals, githubActionsVersionSignals(pkg)...)
		return signals
	}
	age := a.now().UTC().Sub(pkg.PublishedAt.UTC())
	if age < 0 {
		age = 0
	}
	if age <= 72*time.Hour {
		signals = append(signals, models.Signal{
			Type:      "recent_release",
			Severity:  "medium",
			Score:     30,
			Message:   fmt.Sprintf("Version was published recently (%s ago). Review before installing brand-new releases.", humanDuration(age)),
			Source:    "registry",
			CreatedAt: pkg.PublishedAt,
		})
	}
	signals = append(signals, githubActionsVersionSignals(pkg)...)
	return signals
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
		key := vulnerabilityDedupeKey(vuln)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, vuln)
	}
	return out
}

func vulnerabilityDedupeKey(vuln models.Vulnerability) string {
	for _, id := range vuln.GHSAIDs {
		if id != "" {
			return strings.ToUpper(id)
		}
	}
	for _, id := range vuln.CVEIDs {
		if id != "" {
			return strings.ToUpper(id)
		}
	}
	for _, alias := range vuln.Aliases {
		upper := strings.ToUpper(alias)
		if strings.HasPrefix(upper, "GHSA-") || strings.HasPrefix(upper, "CVE-") {
			return upper
		}
	}
	if vuln.ID != "" {
		return strings.ToUpper(vuln.ID)
	}
	return vuln.Summary
}

func containsString(values []string, value string) bool {
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return true
		}
	}
	return false
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

func appendSkippedProviders(left []models.SkippedProvider, right ...models.SkippedProvider) []models.SkippedProvider {
	if len(right) == 0 {
		return left
	}
	out := append([]models.SkippedProvider{}, left...)
	seen := map[string]struct{}{}
	for _, item := range out {
		seen[item.Provider+"\x00"+item.Reason] = struct{}{}
	}
	for _, item := range right {
		key := item.Provider + "\x00" + item.Reason
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, item)
		seen[key] = struct{}{}
	}
	return out
}

func appendUniqueStrings(left []string, right ...string) []string {
	out := append([]string{}, left...)
	for _, value := range right {
		out = appendUniqueString(out, value)
	}
	return out
}

func appendUniqueString(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}

func preferredFixedVersions(vulns []models.Vulnerability, registryVersions []string) []string {
	if len(vulns) == 0 {
		return nil
	}
	registrySet := map[string]struct{}{}
	for _, version := range registryVersions {
		registrySet[version] = struct{}{}
	}
	fixedSet := map[string]struct{}{}
	for _, vuln := range vulns {
		for _, fixed := range vuln.FixedVersions {
			fixed = strings.TrimSpace(fixed)
			if fixed == "" {
				continue
			}
			if _, ok := registrySet[fixed]; !ok {
				continue
			}
			fixedSet[fixed] = struct{}{}
		}
	}
	fixed := make([]string, 0, len(fixedSet))
	for version := range fixedSet {
		fixed = append(fixed, version)
	}
	sort.Slice(fixed, func(i, j int) bool {
		return registry.CompareVersionsForApp(fixed[i], fixed[j]) > 0
	})
	return fixed
}

func advisoryCoverage(checked []string, skipped []models.SkippedProvider, errors []models.ProviderError) (string, string) {
	switch {
	case len(checked) == 0:
		return "none", "no vulnerability provider supports this ecosystem"
	case len(errors) > 0 && len(errors) == len(checked):
		return "error", "all checked vulnerability providers returned errors"
	case len(errors) > 0:
		return "partial", "some vulnerability providers returned errors"
	case len(skipped) > 0:
		return "partial", "some configured vulnerability providers do not support this ecosystem"
	default:
		return "full", "all configured vulnerability providers were checked"
	}
}

func githubActionsVersionSignals(pkg models.PackageVersion) []models.Signal {
	if pkg.Ecosystem != models.EcosystemGitHubActions {
		return nil
	}
	version := strings.TrimSpace(pkg.Version)
	if isFullGitSHA(version) || isExactSemverTag(version) {
		return nil
	}
	if isMajorOnlyGitHubActionTag(version) {
		return []models.Signal{{
			Type:     "mutable_action_tag",
			Severity: "medium",
			Score:    50,
			Message:  "GitHub Actions major-version tags such as v4 are mutable. Prefer a full semver tag or a pinned commit SHA.",
			Source:   "registry",
		}}
	}
	return []models.Signal{{
		Type:     "unpinned_action_ref",
		Severity: "medium",
		Score:    50,
		Message:  "GitHub Actions refs that are not full semver tags or commit SHAs may move. Prefer a full semver tag or a pinned commit SHA.",
		Source:   "registry",
	}}
}

func isFullGitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func isExactSemverTag(value string) bool {
	value = strings.TrimPrefix(value, "v")
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func isMajorOnlyGitHubActionTag(value string) bool {
	value = strings.TrimPrefix(value, "v")
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

func nextAction(recommendation string, vulnCount int, signals []models.Signal, providerErrors []models.ProviderError) string {
	switch recommendation {
	case risk.RecommendationAllow:
		return "install"
	case risk.RecommendationBlock:
		return "do_not_install; use suggest_safe_version or compare_versions to choose a safer version"
	case risk.RecommendationReview:
		if hasSignalType(signals, "recent_release") && vulnCount == 0 {
			return "review_recent_release_before_installing"
		}
		if len(signals) > 0 && vulnCount == 0 {
			return "review_risk_signals_before_installing"
		}
		return "review_advisories_before_installing"
	default:
		if hasGitHubAuthDiagnostic(providerErrors) {
			return "configure_or_verify_github_token_and_retry; or skip/defer; or require_explicit_user_risk_acceptance"
		}
		if len(providerErrors) > 0 {
			return "retry_or_check_manually"
		}
		return "review_before_installing"
	}
}

func hasGitHubAuthDiagnostic(providerErrors []models.ProviderError) bool {
	for _, providerError := range providerErrors {
		if providerError.Provider == "GitHub Advisory DB" && strings.Contains(providerError.Message, "GitHub API access was rate-limited or denied") {
			return true
		}
	}
	return false
}

func hasSignalType(signals []models.Signal, signalType string) bool {
	for _, signal := range signals {
		if signal.Type == signalType {
			return true
		}
	}
	return false
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
