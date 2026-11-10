package app

import (
	"os"
	"path"
	"strings"

	"github.com/clidey/deptrust/internal/models"
	"github.com/clidey/deptrust/internal/risk"
)

// isPrivateGoModule applies Go's GOPRIVATE module-prefix glob semantics.
func isPrivateGoModule(query models.Query) bool {
	if query.Ecosystem != models.EcosystemGo {
		return false
	}
	target := strings.TrimSpace(query.Package)
	for _, glob := range strings.Split(os.Getenv("GOPRIVATE"), ",") {
		glob = strings.TrimSuffix(glob, "/")
		if glob == "" {
			continue
		}
		remaining := strings.Count(glob, "/")
		prefix := target
		for i := 0; i < len(target); i++ {
			if target[i] == '/' {
				if remaining == 0 {
					prefix = target[:i]
					break
				}
				remaining--
			}
		}
		if remaining > 0 {
			continue
		}
		if matched, _ := path.Match(glob, prefix); matched {
			return true
		}
	}
	return false
}

func (a App) privateGoResult(query models.Query) models.CheckResult {
	const reason = "Private Go module matches GOPRIVATE; public registry and advisory requests were skipped. Advisory coverage is unavailable."
	result := models.CheckResult{
		Ecosystem: query.Ecosystem, Package: query.Package, Version: query.Version,
		Recommendation: risk.RecommendationUnknown, Classification: risk.ClassificationUnknown,
		Reason: reason, Summary: reason, NextAction: "check_manually",
		RegistryVerification: "unverified", RegistryVerificationReason: reason,
		AdvisoryCoverage: "none", AdvisoryCoverageReason: reason,
	}
	for _, provider := range a.providers {
		result.SkippedProviders = append(result.SkippedProviders, models.SkippedProvider{Provider: provider.Name(), Reason: reason})
	}
	return result
}
