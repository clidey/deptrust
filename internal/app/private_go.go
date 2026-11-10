package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/clidey/deptrust/internal/models"
	"github.com/clidey/deptrust/internal/risk"
)

// isPrivateGoModule applies Go's GOPRIVATE module-prefix glob semantics.
func isPrivateGoModule(query models.Query, patterns string) bool {
	if query.Ecosystem != models.EcosystemGo {
		return false
	}
	target := strings.TrimSpace(query.Package)
	for _, glob := range strings.Split(patterns, ",") {
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

// goPrivatePatterns delegates saved GOENV settings and precedence to Go itself.
func goPrivatePatterns(ctx context.Context) (string, error) {
	if value := os.Getenv("GOPRIVATE"); value != "" {
		return value, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "env", "GOPRIVATE")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOTELEMETRY=off")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot read effective GOPRIVATE; install Go or export GOPRIVATE before checking Go modules")
	}
	return strings.TrimSpace(string(out)), nil
}

func (a App) unavailableSourceResult(query models.Query, reason string) models.CheckResult {
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
