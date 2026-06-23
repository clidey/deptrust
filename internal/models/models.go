package models

import (
	"fmt"
	"strings"
	"time"
)

const LatestVersion = "latest"

type Ecosystem string

const (
	EcosystemNPM   Ecosystem = "npm"
	EcosystemPyPI  Ecosystem = "pypi"
	EcosystemCargo Ecosystem = "cargo"
)

func NormalizeEcosystem(value string) (Ecosystem, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "npm", "node", "nodejs":
		return EcosystemNPM, nil
	case "pypi", "pip", "python":
		return EcosystemPyPI, nil
	case "cargo", "crate", "crates", "crates.io", "rust":
		return EcosystemCargo, nil
	default:
		return "", fmt.Errorf("unsupported ecosystem %q; supported ecosystems: npm, pypi, cargo", value)
	}
}

func (e Ecosystem) OSVEcosystem() string {
	switch e {
	case EcosystemNPM:
		return "npm"
	case EcosystemPyPI:
		return "PyPI"
	case EcosystemCargo:
		return "crates.io"
	default:
		return string(e)
	}
}

func (e Ecosystem) GitHubEcosystem() string {
	switch e {
	case EcosystemNPM:
		return "npm"
	case EcosystemPyPI:
		return "pip"
	case EcosystemCargo:
		return "rust"
	default:
		return string(e)
	}
}

type Query struct {
	Ecosystem Ecosystem `json:"ecosystem"`
	Package   string    `json:"package"`
	Version   string    `json:"version,omitempty"`
}

func (q Query) Validate() error {
	if strings.TrimSpace(string(q.Ecosystem)) == "" {
		return fmt.Errorf("ecosystem is required")
	}
	if strings.TrimSpace(q.Package) == "" {
		return fmt.Errorf("package is required")
	}
	return nil
}

type PackageVersion struct {
	Ecosystem Ecosystem `json:"ecosystem"`
	Package   string    `json:"package"`
	Version   string    `json:"version"`
	Latest    string    `json:"latest_version,omitempty"`
}

type Reference struct {
	Type string `json:"type,omitempty"`
	URL  string `json:"url"`
}

type Vulnerability struct {
	ID             string      `json:"id"`
	Aliases        []string    `json:"aliases,omitempty"`
	Severity       string      `json:"severity"`
	Summary        string      `json:"summary,omitempty"`
	Details        string      `json:"details,omitempty"`
	Source         string      `json:"source"`
	AffectedRanges []string    `json:"affected_ranges,omitempty"`
	FixedVersions  []string    `json:"fixed_versions,omitempty"`
	References     []Reference `json:"references,omitempty"`
	PublishedAt    *time.Time  `json:"published_at,omitempty"`
	ModifiedAt     *time.Time  `json:"modified_at,omitempty"`
}

type ProviderError struct {
	Provider string `json:"provider"`
	Message  string `json:"message"`
}

type CheckResult struct {
	Ecosystem                  Ecosystem       `json:"ecosystem"`
	Package                    string          `json:"package"`
	Version                    string          `json:"version"`
	LatestVersion              string          `json:"latest_version,omitempty"`
	KnownVulnerabilitiesFound  bool            `json:"known_vulnerabilities_found"`
	SafeToUse                  bool            `json:"safe_to_use"`
	RiskScore                  int             `json:"risk_score"`
	Classification             string          `json:"classification"`
	Recommendation             string          `json:"recommendation"`
	Summary                    string          `json:"summary"`
	Vulnerabilities            []Vulnerability `json:"vulnerabilities"`
	ProviderErrors             []ProviderError `json:"provider_errors,omitempty"`
	ResolvedFromVersionRequest string          `json:"resolved_from_version_request,omitempty"`
}

type SuggestResult struct {
	Ecosystem             Ecosystem       `json:"ecosystem"`
	Package               string          `json:"package"`
	LatestVersion         string          `json:"latest_version,omitempty"`
	SuggestedVersion      string          `json:"suggested_version,omitempty"`
	Recommendation        string          `json:"recommendation"`
	Summary               string          `json:"summary"`
	CheckedVersions       []string        `json:"checked_versions,omitempty"`
	LatestVersionResult   *CheckResult    `json:"latest_version_result,omitempty"`
	SuggestedVersionCheck *CheckResult    `json:"suggested_version_check,omitempty"`
	ProviderErrors        []ProviderError `json:"provider_errors,omitempty"`
}
