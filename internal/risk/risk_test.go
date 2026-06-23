package risk

import (
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestScoreNoKnownVulnerabilitiesAllows(t *testing.T) {
	got := Score(pkg(), nil, nil, nil)
	if got.Recommendation != RecommendationAllow {
		t.Fatalf("recommendation = %q, want %q", got.Recommendation, RecommendationAllow)
	}
	if !got.SafeToUse {
		t.Fatal("SafeToUse = false, want true")
	}
	if got.RiskScore != 0 {
		t.Fatalf("RiskScore = %d, want 0", got.RiskScore)
	}
}

func TestScoreHighVulnerabilityBlocks(t *testing.T) {
	got := Score(pkg(), []models.Vulnerability{{Severity: "high"}}, nil, nil)
	if got.Recommendation != RecommendationBlock {
		t.Fatalf("recommendation = %q, want %q", got.Recommendation, RecommendationBlock)
	}
	if got.SafeToUse {
		t.Fatal("SafeToUse = true, want false")
	}
	if got.RiskScore != 80 {
		t.Fatalf("RiskScore = %d, want 80", got.RiskScore)
	}
}

func TestScoreMediumVulnerabilityReviews(t *testing.T) {
	got := Score(pkg(), []models.Vulnerability{{Severity: "medium"}}, nil, nil)
	if got.Recommendation != RecommendationReview {
		t.Fatalf("recommendation = %q, want %q", got.Recommendation, RecommendationReview)
	}
}

func TestScoreLowVulnerabilityAllows(t *testing.T) {
	got := Score(pkg(), []models.Vulnerability{{Severity: "low"}}, nil, nil)
	if got.Recommendation != RecommendationAllow {
		t.Fatalf("recommendation = %q, want %q", got.Recommendation, RecommendationAllow)
	}
	if !got.SafeToUse {
		t.Fatal("SafeToUse = false, want true")
	}
}

func TestScoreProviderFailureReturnsUnknown(t *testing.T) {
	got := Score(pkg(), nil, nil, []models.ProviderError{{Provider: "OSV", Message: "timeout"}})
	if got.Recommendation != RecommendationUnknown {
		t.Fatalf("recommendation = %q, want %q", got.Recommendation, RecommendationUnknown)
	}
	if got.SafeToUse {
		t.Fatal("SafeToUse = true, want false")
	}
}

func TestScoreRecentReleaseSignalReviews(t *testing.T) {
	got := Score(pkg(), nil, []models.Signal{{Severity: "medium", Score: 30}}, nil)
	if got.Recommendation != RecommendationReview {
		t.Fatalf("recommendation = %q, want %q", got.Recommendation, RecommendationReview)
	}
	if got.Classification != ClassificationSuspicious {
		t.Fatalf("classification = %q, want %q", got.Classification, ClassificationSuspicious)
	}
}

func pkg() models.PackageVersion {
	return models.PackageVersion{
		Ecosystem: models.EcosystemNPM,
		Package:   "lodash",
		Version:   "4.17.20",
	}
}
