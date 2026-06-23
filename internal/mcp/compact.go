package mcp

import (
	"strings"
	"time"

	"github.com/clidey/deptrust/internal/models"
)

type compactCheckResult struct {
	Ecosystem                  models.Ecosystem         `json:"ecosystem"`
	Package                    string                   `json:"package"`
	Version                    string                   `json:"version"`
	LatestVersion              string                   `json:"latest_version,omitempty"`
	PublishedAt                *time.Time               `json:"published_at,omitempty"`
	KnownVulnerabilitiesFound  bool                     `json:"known_vulnerabilities_found"`
	SafeToUse                  bool                     `json:"safe_to_use"`
	ShouldInstall              bool                     `json:"should_install"`
	RiskScore                  int                      `json:"risk_score"`
	Classification             string                   `json:"classification"`
	Recommendation             string                   `json:"recommendation"`
	Reason                     string                   `json:"reason"`
	NextAction                 string                   `json:"next_action"`
	Summary                    string                   `json:"summary"`
	VulnerabilityCount         int                      `json:"vulnerability_count"`
	VulnerabilityCounts        vulnerabilityCounts      `json:"vulnerability_counts"`
	HighestSeverity            string                   `json:"highest_severity,omitempty"`
	Signals                    []models.Signal          `json:"signals,omitempty"`
	ProviderErrors             []models.ProviderError   `json:"provider_errors,omitempty"`
	CheckedProviders           []string                 `json:"checked_providers,omitempty"`
	SkippedProviders           []models.SkippedProvider `json:"skipped_providers,omitempty"`
	AdvisoryCoverage           string                   `json:"advisory_coverage"`
	AdvisoryCoverageReason     string                   `json:"advisory_coverage_reason,omitempty"`
	ResolvedFromVersionRequest string                   `json:"resolved_from_version_request,omitempty"`
	FullResponseCommand        string                   `json:"full_response_command,omitempty"`
}

type compactSuggestResult struct {
	Ecosystem             models.Ecosystem         `json:"ecosystem"`
	Package               string                   `json:"package"`
	LatestVersion         string                   `json:"latest_version,omitempty"`
	SuggestedVersion      string                   `json:"suggested_version,omitempty"`
	Recommendation        string                   `json:"recommendation"`
	Summary               string                   `json:"summary"`
	CheckedVersionCount   int                      `json:"checked_version_count"`
	SafeAlternatives      []string                 `json:"safe_alternatives,omitempty"`
	LatestVersionResult   *compactCheckResult      `json:"latest_version_result,omitempty"`
	SuggestedVersionCheck *compactCheckResult      `json:"suggested_version_check,omitempty"`
	ProviderErrors        []models.ProviderError   `json:"provider_errors,omitempty"`
	CheckedProviders      []string                 `json:"checked_providers,omitempty"`
	SkippedProviders      []models.SkippedProvider `json:"skipped_providers,omitempty"`
	FullResponseCommand   string                   `json:"full_response_command,omitempty"`
}

type compactCompareResult struct {
	Ecosystem                  models.Ecosystem   `json:"ecosystem"`
	Package                    string             `json:"package"`
	FromVersion                string             `json:"from_version"`
	ToVersion                  string             `json:"to_version"`
	ImprovesRisk               bool               `json:"improves_risk"`
	Recommendation             string             `json:"recommendation"`
	Summary                    string             `json:"summary"`
	NextAction                 string             `json:"next_action"`
	From                       compactCheckResult `json:"from"`
	To                         compactCheckResult `json:"to"`
	ResolvedVulnerabilityCount int                `json:"resolved_vulnerability_count"`
	AddedVulnerabilityCount    int                `json:"added_vulnerability_count"`
	FullResponseCommand        string             `json:"full_response_command,omitempty"`
}

type vulnerabilityCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Unknown  int `json:"unknown"`
}

func compactCheck(result models.CheckResult) compactCheckResult {
	return compactCheckResult{
		Ecosystem:                  result.Ecosystem,
		Package:                    result.Package,
		Version:                    result.Version,
		LatestVersion:              result.LatestVersion,
		PublishedAt:                result.PublishedAt,
		KnownVulnerabilitiesFound:  result.KnownVulnerabilitiesFound,
		SafeToUse:                  result.SafeToUse,
		ShouldInstall:              result.ShouldInstall,
		RiskScore:                  result.RiskScore,
		Classification:             result.Classification,
		Recommendation:             result.Recommendation,
		Reason:                     result.Reason,
		NextAction:                 result.NextAction,
		Summary:                    result.Summary,
		VulnerabilityCount:         len(result.Vulnerabilities),
		VulnerabilityCounts:        countSeverities(result.Vulnerabilities),
		HighestSeverity:            highestSeverity(result.Vulnerabilities),
		Signals:                    result.Signals,
		ProviderErrors:             result.ProviderErrors,
		CheckedProviders:           result.CheckedProviders,
		SkippedProviders:           result.SkippedProviders,
		AdvisoryCoverage:           result.AdvisoryCoverage,
		AdvisoryCoverageReason:     result.AdvisoryCoverageReason,
		ResolvedFromVersionRequest: result.ResolvedFromVersionRequest,
		FullResponseCommand:        command("check", string(result.Ecosystem), result.Package, result.Version),
	}
}

func compactSuggest(result models.SuggestResult) compactSuggestResult {
	var latest *compactCheckResult
	if result.LatestVersionResult != nil {
		compact := compactCheck(*result.LatestVersionResult)
		latest = &compact
	}

	var suggested *compactCheckResult
	if result.SuggestedVersionCheck != nil {
		compact := compactCheck(*result.SuggestedVersionCheck)
		suggested = &compact
	}

	return compactSuggestResult{
		Ecosystem:             result.Ecosystem,
		Package:               result.Package,
		LatestVersion:         result.LatestVersion,
		SuggestedVersion:      result.SuggestedVersion,
		Recommendation:        result.Recommendation,
		Summary:               result.Summary,
		CheckedVersionCount:   len(result.CheckedVersions),
		SafeAlternatives:      result.SafeAlternatives,
		LatestVersionResult:   latest,
		SuggestedVersionCheck: suggested,
		ProviderErrors:        result.ProviderErrors,
		CheckedProviders:      result.CheckedProviders,
		SkippedProviders:      result.SkippedProviders,
		FullResponseCommand:   command("suggest", string(result.Ecosystem), result.Package),
	}
}

func compactCompare(result models.CompareResult) compactCompareResult {
	return compactCompareResult{
		Ecosystem:                  result.Ecosystem,
		Package:                    result.Package,
		FromVersion:                result.FromVersion,
		ToVersion:                  result.ToVersion,
		ImprovesRisk:               result.ImprovesRisk,
		Recommendation:             result.Recommendation,
		Summary:                    result.Summary,
		NextAction:                 result.NextAction,
		From:                       compactCheck(result.From),
		To:                         compactCheck(result.To),
		ResolvedVulnerabilityCount: len(result.ResolvedVulnerabilities),
		AddedVulnerabilityCount:    len(result.AddedVulnerabilities),
		FullResponseCommand:        command("compare", string(result.Ecosystem), result.Package, result.FromVersion, result.ToVersion),
	}
}

func countSeverities(vulns []models.Vulnerability) vulnerabilityCounts {
	var counts vulnerabilityCounts
	for _, vuln := range vulns {
		switch vuln.Severity {
		case "critical":
			counts.Critical++
		case "high":
			counts.High++
		case "medium":
			counts.Medium++
		case "low":
			counts.Low++
		default:
			counts.Unknown++
		}
	}
	return counts
}

func highestSeverity(vulns []models.Vulnerability) string {
	highest := ""
	highestRank := 0
	for _, vuln := range vulns {
		rank := severityRank(vuln.Severity)
		if rank > highestRank {
			highest = vuln.Severity
			highestRank = rank
		}
	}
	return highest
}

func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func command(name string, args ...string) string {
	parts := []string{"deptrust", name, "--json"}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	if !strings.ContainsAny(arg, " \t\n'\"\\$`") {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}
