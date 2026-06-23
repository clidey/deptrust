package risk

import (
	"fmt"
	"strings"

	"github.com/clidey/deptrust/internal/models"
)

const (
	ClassificationNoKnownVulns = "no_known_vulnerabilities"
	ClassificationVulnerable   = "vulnerable"
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

func Score(pkg models.PackageVersion, vulns []models.Vulnerability, providerErrors []models.ProviderError) Assessment {
	if len(vulns) == 0 {
		if len(providerErrors) > 0 {
			return Assessment{
				RiskScore:      0,
				Classification: ClassificationUnknown,
				Recommendation: RecommendationUnknown,
				SafeToUse:      false,
				Summary:        fmt.Sprintf("Could not fully assess %s %s because vulnerability providers returned errors.", pkg.Package, pkg.Version),
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

	recommendation := RecommendationReview
	switch {
	case maxScore >= 80:
		recommendation = RecommendationBlock
	case maxScore <= 20:
		recommendation = RecommendationAllow
	}

	return Assessment{
		RiskScore:      maxScore,
		Classification: ClassificationVulnerable,
		Recommendation: recommendation,
		SafeToUse:      recommendation == RecommendationAllow,
		Summary:        vulnerabilitySummary(pkg, len(vulns), maxSeverity, recommendation),
	}
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

func vulnerabilitySummary(pkg models.PackageVersion, count int, maxSeverity string, recommendation string) string {
	vulnWord := "vulnerability"
	if count != 1 {
		vulnWord = "vulnerabilities"
	}

	if recommendation == RecommendationBlock {
		return fmt.Sprintf("%s %s has %d known %s, including %s severity. Block this exact version and prefer a fixed release.", pkg.Package, pkg.Version, count, vulnWord, maxSeverity)
	}
	if recommendation == RecommendationAllow {
		return fmt.Sprintf("%s %s has %d known low severity %s. It is not blocked by the default policy, but review the advisory if this package is security-sensitive.", pkg.Package, pkg.Version, count, vulnWord)
	}

	return fmt.Sprintf("%s %s has %d known %s, including %s severity. Review before installing this exact version.", pkg.Package, pkg.Version, count, vulnWord, maxSeverity)
}
