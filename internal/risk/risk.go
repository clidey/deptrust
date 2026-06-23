package risk

import (
	"fmt"
	"strings"

	"github.com/clidey/deptrust/internal/models"
)

const (
	ClassificationNoKnownVulns = "no_known_vulnerabilities"
	ClassificationVulnerable   = "vulnerable"
	ClassificationSuspicious   = "suspicious"
	ClassificationUnknown      = "unknown"

	RecommendationAllow   = "allow"
	RecommendationReview  = "review"
	RecommendationBlock   = "block"
	RecommendationUnknown = "unknown"
)

type Assessment struct {
	RiskScore      int
	Classification string
	Recommendation string
	SafeToUse      bool
	Summary        string
}

func Score(pkg models.PackageVersion, vulns []models.Vulnerability, signals []models.Signal, providerErrors []models.ProviderError) Assessment {
	if len(vulns) == 0 && len(signals) == 0 {
		if len(providerErrors) > 0 {
			summary := fmt.Sprintf("Could not fully assess %s %s because vulnerability providers returned errors.", pkg.Package, pkg.Version)
			if hasUnsupportedProviderCoverage(providerErrors) {
				summary = fmt.Sprintf("Could not fully assess %s %s because no vulnerability provider supports this ecosystem.", pkg.Package, pkg.Version)
			}
			return Assessment{
				RiskScore:      0,
				Classification: ClassificationUnknown,
				Recommendation: RecommendationUnknown,
				SafeToUse:      false,
				Summary:        summary,
			}
		}

		return Assessment{
			RiskScore:      0,
			Classification: ClassificationNoKnownVulns,
			Recommendation: RecommendationAllow,
			SafeToUse:      true,
			Summary:        fmt.Sprintf("No known vulnerabilities were found for %s %s.", pkg.Package, pkg.Version),
		}
	}

	maxSeverity := "unknown"
	maxScore := 0
	for _, vuln := range vulns {
		score := severityScore(vuln.Severity)
		if score > maxScore {
			maxScore = score
			maxSeverity = normalizeSeverity(vuln.Severity)
		}
	}
	for _, signal := range signals {
		if signal.Score > maxScore {
			maxScore = signal.Score
			maxSeverity = normalizeSeverity(signal.Severity)
		}
	}

	recommendation := RecommendationReview
	switch {
	case maxScore >= 80:
		recommendation = RecommendationBlock
	case maxScore <= 20:
		recommendation = RecommendationAllow
	}

	return Assessment{
		RiskScore:      maxScore,
		Classification: classificationFor(vulns, signals),
		Recommendation: recommendation,
		SafeToUse:      recommendation == RecommendationAllow,
		Summary:        assessmentSummary(pkg, len(vulns), len(signals), maxSeverity, recommendation),
	}
}

func hasUnsupportedProviderCoverage(providerErrors []models.ProviderError) bool {
	for _, item := range providerErrors {
		if item.Provider == "deptrust" && strings.Contains(item.Message, "no vulnerability provider supports") {
			return true
		}
	}
	return false
}

func SeverityRank(severity string) int {
	switch normalizeSeverity(severity) {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium", "moderate":
		return 3
	case "low":
		return 2
	default:
		return 1
	}
}

func NormalizeSeverity(severity string) string {
	return normalizeSeverity(severity)
}

func severityScore(severity string) int {
	switch normalizeSeverity(severity) {
	case "critical":
		return 95
	case "high":
		return 80
	case "medium", "moderate":
		return 50
	case "low":
		return 20
	default:
		return 40
	}
}

func classificationFor(vulns []models.Vulnerability, signals []models.Signal) string {
	if len(vulns) > 0 {
		return ClassificationVulnerable
	}
	if len(signals) > 0 {
		return ClassificationSuspicious
	}
	return ClassificationNoKnownVulns
}

func normalizeSeverity(severity string) string {
	normalized := strings.ToLower(strings.TrimSpace(severity))
	switch normalized {
	case "":
		return "unknown"
	case "moderate":
		return "medium"
	default:
		return normalized
	}
}

func assessmentSummary(pkg models.PackageVersion, vulnCount, signalCount int, maxSeverity string, recommendation string) string {
	if vulnCount == 0 && signalCount > 0 {
		return fmt.Sprintf("%s %s has no known vulnerabilities, but %d risk signal was found. Review before installing this exact version.", pkg.Package, pkg.Version, signalCount)
	}

	vulnWord := "vulnerability"
	if vulnCount != 1 {
		vulnWord = "vulnerabilities"
	}

	if recommendation == RecommendationBlock {
		return fmt.Sprintf("%s %s has %d known %s, including %s severity. Block this exact version and prefer a fixed release.", pkg.Package, pkg.Version, vulnCount, vulnWord, maxSeverity)
	}
	if recommendation == RecommendationAllow {
		return fmt.Sprintf("%s %s has %d known low severity %s. It is not blocked by the default policy, but review the advisory if this package is security-sensitive.", pkg.Package, pkg.Version, vulnCount, vulnWord)
	}

	return fmt.Sprintf("%s %s has %d known %s, including %s severity. Review before installing this exact version.", pkg.Package, pkg.Version, vulnCount, vulnWord, maxSeverity)
}
