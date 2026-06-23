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
	EcosystemGo    Ecosystem = "go"
	EcosystemRuby  Ecosystem = "rubygems"
	EcosystemNuGet Ecosystem = "nuget"
	EcosystemMaven Ecosystem = "maven"
)

func NormalizeEcosystem(value string) (Ecosystem, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "npm", "node", "nodejs":
		return EcosystemNPM, nil
	case "pypi", "pip", "python":
		return EcosystemPyPI, nil
	case "cargo", "crate", "crates", "crates.io", "rust":
		return EcosystemCargo, nil
	case "go", "golang", "gomod", "go module", "go modules":
		return EcosystemGo, nil
	case "ruby", "gem", "gems", "rubygem", "rubygems":
		return EcosystemRuby, nil
	case "nuget", ".net", "dotnet":
		return EcosystemNuGet, nil
	case "maven", "java", "jvm":
		return EcosystemMaven, nil
	default:
		return "", fmt.Errorf("unsupported ecosystem %q; supported ecosystems: npm, pypi, cargo, go, rubygems, nuget, maven", value)
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
	case EcosystemGo:
		return "Go"
	case EcosystemRuby:
		return "RubyGems"
	case EcosystemNuGet:
		return "NuGet"
	case EcosystemMaven:
		return "Maven"
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
	case EcosystemGo:
		return "go"
	case EcosystemRuby:
		return "rubygems"
	case EcosystemNuGet:
		return "nuget"
	case EcosystemMaven:
		return "maven"
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
	Ecosystem   Ecosystem  `json:"ecosystem"`
	Package     string     `json:"package"`
	Version     string     `json:"version"`
	Latest      string     `json:"latest_version,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
}

type Reference struct {
	Type string `json:"type,omitempty"`
	URL  string `json:"url"`
}

type Vulnerability struct {
	ID             string      `json:"id"`
	Aliases        []string    `json:"aliases,omitempty"`
	CVEIDs         []string    `json:"cve_ids,omitempty"`
	GHSAIDs        []string    `json:"ghsa_ids,omitempty"`
	Severity       string      `json:"severity"`
	Summary        string      `json:"summary,omitempty"`
	Details        string      `json:"details,omitempty"`
	Source         string      `json:"source"`
	AdvisoryURL    string      `json:"advisory_url,omitempty"`
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

type Signal struct {
	Type      string     `json:"type"`
	Severity  string     `json:"severity"`
	Score     int        `json:"score"`
	Message   string     `json:"message"`
	Source    string     `json:"source,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

type CheckResult struct {
	Ecosystem                  Ecosystem       `json:"ecosystem"`
	Package                    string          `json:"package"`
	Version                    string          `json:"version"`
	LatestVersion              string          `json:"latest_version,omitempty"`
	PublishedAt                *time.Time      `json:"published_at,omitempty"`
	KnownVulnerabilitiesFound  bool            `json:"known_vulnerabilities_found"`
	SafeToUse                  bool            `json:"safe_to_use"`
	ShouldInstall              bool            `json:"should_install"`
	RiskScore                  int             `json:"risk_score"`
	Classification             string          `json:"classification"`
	Recommendation             string          `json:"recommendation"`
	Reason                     string          `json:"reason"`
	NextAction                 string          `json:"next_action"`
	Summary                    string          `json:"summary"`
	Signals                    []Signal        `json:"signals,omitempty"`
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
	SafeAlternatives      []string        `json:"safe_alternatives,omitempty"`
	LatestVersionResult   *CheckResult    `json:"latest_version_result,omitempty"`
	SuggestedVersionCheck *CheckResult    `json:"suggested_version_check,omitempty"`
	ProviderErrors        []ProviderError `json:"provider_errors,omitempty"`
}

type CompareResult struct {
	Ecosystem               Ecosystem       `json:"ecosystem"`
	Package                 string          `json:"package"`
	FromVersion             string          `json:"from_version"`
	ToVersion               string          `json:"to_version"`
	ImprovesRisk            bool            `json:"improves_risk"`
	Recommendation          string          `json:"recommendation"`
	Summary                 string          `json:"summary"`
	NextAction              string          `json:"next_action"`
	From                    CheckResult     `json:"from"`
	To                      CheckResult     `json:"to"`
	ResolvedVulnerabilities []Vulnerability `json:"resolved_vulnerabilities,omitempty"`
	AddedVulnerabilities    []Vulnerability `json:"added_vulnerabilities,omitempty"`
}
