package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestCompactCheckOmitsVulnerabilityArrays(t *testing.T) {
	result := models.CheckResult{
		Ecosystem:                  models.EcosystemNPM,
		Package:                    "vite",
		Version:                    "7.0.0",
		RiskScore:                  80,
		Classification:             "vulnerable",
		Recommendation:             "block",
		Reason:                     "Found known vulnerabilities.",
		NextAction:                 "do_not_install",
		Summary:                    "vite 7.0.0 has known vulnerabilities.",
		RegistryVerification:       "unverified",
		RegistryVerificationReason: "registry timed out",
		Vulnerabilities: []models.Vulnerability{
			{
				ID:             "GHSA-test-1234-5678",
				CVEIDs:         []string{"CVE-2026-0001"},
				GHSAIDs:        []string{"GHSA-test-1234-5678"},
				Severity:       "high",
				Summary:        "test advisory",
				Details:        "large markdown advisory body",
				Source:         "GitHub Advisory DB",
				AdvisoryURL:    "https://github.com/advisories/GHSA-test-1234-5678",
				AffectedRanges: []string{">= 7.0.0, < 7.3.5"},
				FixedVersions:  []string{"7.3.5"},
				References: []models.Reference{
					{Type: "REFERENCE", URL: "https://example.com/large-reference"},
				},
			},
			{
				ID:       "GHSA-test-low",
				Severity: "low",
				Source:   "OSV",
			},
		},
	}

	encoded, err := json.Marshal(compactCheck(result))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	text := string(encoded)

	if strings.Contains(text, `"vulnerabilities":`) || strings.Contains(text, "details") || strings.Contains(text, "references") {
		t.Fatalf("compact MCP result included vulnerability fields: %s", text)
	}
	if !strings.Contains(text, `"vulnerability_count":2`) {
		t.Fatalf("compact MCP result missing vulnerability count: %s", text)
	}
	if !strings.Contains(text, `"high":1`) || !strings.Contains(text, `"low":1`) {
		t.Fatalf("compact MCP result missing severity counts: %s", text)
	}
	if !strings.Contains(text, `"highest_severity":"high"`) {
		t.Fatalf("compact MCP result missing highest severity: %s", text)
	}
	if !strings.Contains(text, `"registry_verification":"unverified"`) || !strings.Contains(text, `"registry_verification_reason":"registry timed out"`) {
		t.Fatalf("compact MCP result missing registry verification: %s", text)
	}
	if !strings.Contains(text, `"full_response_command":"deptrust check --json npm vite 7.0.0"`) {
		t.Fatalf("compact MCP result missing full response command: %s", text)
	}
}

func TestCompactCompareOmitsVulnerabilityArrays(t *testing.T) {
	result := models.CompareResult{
		From: models.CheckResult{Vulnerabilities: []models.Vulnerability{{ID: "GHSA-from", Severity: "high"}}},
		To:   models.CheckResult{Vulnerabilities: []models.Vulnerability{{ID: "GHSA-to", Severity: "low"}}},
		ResolvedVulnerabilities: []models.Vulnerability{
			{ID: "GHSA-resolved", Severity: "high"},
		},
		AddedVulnerabilities: []models.Vulnerability{
			{ID: "GHSA-added", Severity: "low"},
		},
	}

	encoded, err := json.Marshal(compactCompare(result))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	text := string(encoded)

	if strings.Contains(text, "resolved_vulnerabilities") || strings.Contains(text, "added_vulnerabilities") {
		t.Fatalf("compact MCP compare result included vulnerability arrays: %s", text)
	}
	if !strings.Contains(text, `"resolved_vulnerability_count":1`) {
		t.Fatalf("compact MCP compare result missing resolved count: %s", text)
	}
	if !strings.Contains(text, `"added_vulnerability_count":1`) {
		t.Fatalf("compact MCP compare result missing added count: %s", text)
	}
}

func TestCountSeveritiesIncludesUnknown(t *testing.T) {
	got := countSeverities([]models.Vulnerability{
		{Severity: "critical"},
		{Severity: "high"},
		{Severity: "medium"},
		{Severity: "low"},
		{Severity: ""},
	})

	if got.Critical != 1 || got.High != 1 || got.Medium != 1 || got.Low != 1 || got.Unknown != 1 {
		t.Fatalf("countSeverities() = %#v, want one of each severity", got)
	}
}

func TestCompactToolTextMentionsFullResponseOnlyWhenRequested(t *testing.T) {
	got := compactToolText("vite 7.0.0 has known vulnerabilities.", "deptrust check --json npm vite 7.0.0")
	if !strings.Contains(got, "compact safety result by default") {
		t.Fatalf("compactToolText() missing compact result note: %q", got)
	}
	if !strings.Contains(got, "If the user asks to see full advisory details") {
		t.Fatalf("compactToolText() missing user-requested full details note: %q", got)
	}
	if !strings.Contains(got, "deptrust check --json npm vite 7.0.0") {
		t.Fatalf("compactToolText() missing full response command: %q", got)
	}
}

func TestCommandQuotesShellArgumentsWhenNeeded(t *testing.T) {
	got := command("check", "npm", "@scope/pkg name", "1.0.0")
	want := "deptrust check --json npm '@scope/pkg name' 1.0.0"
	if got != want {
		t.Fatalf("command() = %q, want %q", got, want)
	}
}
