package osv

import (
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestConvertVulnerabilityUsesDatabaseSeverityAndFixedVersions(t *testing.T) {
	raw := vulnerability{
		ID:      "GHSA-test",
		Summary: "test vuln",
		Aliases: []string{"CVE-2026-0001"},
	}
	raw.DatabaseSpecific.Severity = "HIGH"
	raw.Affected = []affected{
		{
			Ranges: []affectedRange{
				{
					Type: "SEMVER",
					Events: []rangeEvent{
						{Introduced: "0"},
						{Fixed: "1.2.3"},
					},
				},
			},
		},
	}

	got := convertVulnerability(raw, models.PackageVersion{
		Ecosystem: models.EcosystemNPM,
		Package:   "pkg",
		Version:   "1.2.2",
	})

	if got.Severity != "high" {
		t.Fatalf("Severity = %q, want high", got.Severity)
	}
	if len(got.FixedVersions) != 1 || got.FixedVersions[0] != "1.2.3" {
		t.Fatalf("FixedVersions = %#v, want [1.2.3]", got.FixedVersions)
	}
	if len(got.AffectedRanges) != 1 {
		t.Fatalf("AffectedRanges = %#v, want one range", got.AffectedRanges)
	}
}
