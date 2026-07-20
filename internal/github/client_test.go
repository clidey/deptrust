package github

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestQueryReportsActionableRateLimitDiagnostic(t *testing.T) {
	client := New(doFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}))
	_, err := client.Query(context.Background(), models.PackageVersion{
		Ecosystem: models.EcosystemNPM,
		Package:   "pkg",
		Version:   "1.0.0",
	})
	if err == nil || !strings.Contains(err.Error(), "GitHub API access was rate-limited or denied") || strings.Contains(err.Error(), "Bearer") {
		t.Fatalf("Query() error = %v, want actionable redacted diagnostic", err)
	}
}

func TestConvertAdvisoryIncludesIdentifiersAndPatch(t *testing.T) {
	raw := advisory{
		GHSAID:      "GHSA-test-1234-5678",
		CVEID:       "CVE-2026-0001",
		HTMLURL:     "https://github.com/advisories/GHSA-test-1234-5678",
		Summary:     "test advisory",
		Description: "first paragraph\n\nsecond paragraph",
		Severity:    "HIGH",
	}
	raw.Identifiers = []identifier{
		{Type: "GHSA", Value: "GHSA-test-1234-5678"},
		{Type: "CVE", Value: "CVE-2026-0001"},
	}
	raw.Vulnerabilities = []advisoryVulnerability{{}}
	raw.Vulnerabilities[0].Package.Ecosystem = "npm"
	raw.Vulnerabilities[0].Package.Name = "pkg"
	raw.Vulnerabilities[0].VulnerableVersionRange = "< 1.2.3"
	raw.Vulnerabilities[0].FirstPatchedVersion.Identifier = "1.2.3"

	got := convertAdvisory(raw, models.PackageVersion{
		Ecosystem: models.EcosystemNPM,
		Package:   "pkg",
		Version:   "1.2.2",
	})

	if got.Source != "GitHub Advisory DB" {
		t.Fatalf("Source = %q, want GitHub Advisory DB", got.Source)
	}
	if got.Severity != "high" {
		t.Fatalf("Severity = %q, want high", got.Severity)
	}
	if len(got.CVEIDs) != 1 || got.CVEIDs[0] != "CVE-2026-0001" {
		t.Fatalf("CVEIDs = %#v, want CVE", got.CVEIDs)
	}
	if len(got.GHSAIDs) != 1 || got.GHSAIDs[0] != "GHSA-test-1234-5678" {
		t.Fatalf("GHSAIDs = %#v, want GHSA", got.GHSAIDs)
	}
	if len(got.FixedVersions) != 1 || got.FixedVersions[0] != "1.2.3" {
		t.Fatalf("FixedVersions = %#v, want [1.2.3]", got.FixedVersions)
	}
	if got.AdvisoryURL != raw.HTMLURL {
		t.Fatalf("AdvisoryURL = %q, want %q", got.AdvisoryURL, raw.HTMLURL)
	}
}

func TestAdvisoryDecodeAcceptsStringFirstPatchedVersion(t *testing.T) {
	raw := []byte(`[
		{
			"ghsa_id": "GHSA-test-1234-5678",
			"vulnerabilities": [
				{
					"package": {
						"ecosystem": "npm",
						"name": "pkg"
					},
					"vulnerable_version_range": "< 1.2.3",
					"first_patched_version": "1.2.3"
				}
			]
		}
	]`)

	var decoded []advisory
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := decoded[0].Vulnerabilities[0].FirstPatchedVersion.Identifier; got != "1.2.3" {
		t.Fatalf("FirstPatchedVersion.Identifier = %q, want 1.2.3", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type doFunc func(*http.Request) (*http.Response, error)

func (f doFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }
