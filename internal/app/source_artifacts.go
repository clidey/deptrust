package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/clidey/deptrust/internal/models"
)

const unsupportedArtifactSource = "Selected source artifact does not exclusively establish this package from the supported public registry; public registry and advisory requests were skipped. Assessment is unavailable."

func artifactRestriction(ctx context.Context, query models.Query) (string, error) {
	switch query.Ecosystem {
	case models.EcosystemHex:
		return hexArtifactRestriction(query)
	case models.EcosystemCocoaPods:
		return cocoapodsArtifactRestriction(query)
	case models.EcosystemHackage:
		return cabalArtifactRestriction(query)
	case models.EcosystemMaven:
		return mavenArtifactRestriction(query)
	case models.EcosystemNPM:
		return javascriptArtifactRestriction(ctx, query)
	case models.EcosystemPyPI:
		return pythonArtifactRestriction(ctx, query)
	case models.EcosystemPub:
		return dartRestriction(query)
	case models.EcosystemPackagist:
		return composerRestriction(query)
	default:
		return "", fmt.Errorf("source_file is not supported for this ecosystem")
	}
}
func readArtifactJSON(file string) (map[string]any, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("cannot read selected source artifact")
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("invalid source JSON")
	}
	return root, nil
}
func composerRestriction(query models.Query) (string, error) {
	root, err := readArtifactJSON(query.SourceFile)
	if err != nil {
		return "", err
	}
	raw, exists := root["repositories"]
	if !exists {
		return "", nil
	}
	var repos []any
	switch value := raw.(type) {
	case []any:
		repos = value
	case map[string]any:
		for key, repo := range value {
			if key == "packagist.org" && repo == false {
				return unsupportedArtifactSource, nil
			}
			repos = append(repos, repo)
		}
	default:
		return "", fmt.Errorf("invalid Composer repositories")
	}
	for _, raw := range repos {
		repo := table(raw)
		if repo == nil {
			return "", fmt.Errorf("invalid Composer repository")
		}
		if disabled, ok := repo["packagist.org"].(bool); ok && !disabled {
			return unsupportedArtifactSource, nil
		}
		if _, disabled := repo["packagist.org"]; disabled {
			continue
		}
		if only, exists := repo["only"]; exists {
			matched, err := composerPatterns(only, query.Package)
			if err != nil {
				return "", err
			}
			if !matched {
				continue
			}
		}
		if exclude, exists := repo["exclude"]; exists {
			matched, err := composerPatterns(exclude, query.Package)
			if err != nil {
				return "", err
			}
			if matched {
				continue
			}
		}
		if text(repo["type"]) == "composer" && publicSource(text(repo["url"]), "repo.packagist.org", "/") {
			continue
		}
		// Even noncanonical repositories can supply a different package identity.
		return unsupportedArtifactSource, nil
	}
	return "", nil
}
func composerPatterns(raw any, name string) (bool, error) {
	patterns, ok := raw.([]any)
	if !ok {
		return false, fmt.Errorf("invalid Composer repository filter")
	}
	for _, raw := range patterns {
		pattern, ok := raw.(string)
		if !ok {
			return false, fmt.Errorf("invalid Composer repository pattern")
		}
		expression := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$"
		matched, err := regexp.MatchString(expression, name)
		if err != nil {
			return false, fmt.Errorf("invalid Composer repository pattern")
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}
