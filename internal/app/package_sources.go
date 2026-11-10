package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/clidey/deptrust/internal/models"
)

// sourceRestriction checks only explicitly selected package-manager configuration.
// Configuration failures stop assessment before any public request.
func sourceRestriction(ctx context.Context, query models.Query) (string, error) {
	selected := 0
	for _, value := range []string{query.ProjectDir, query.NuGetConfig, query.BundlerLock, query.SourceFile} {
		if value != "" {
			selected++
		}
	}
	if selected > 1 {
		return "", fmt.Errorf("select only one source context")
	}
	if query.Ecosystem == models.EcosystemPyPI {
		reason, err := pythonEnvironmentRestriction()
		if err != nil || reason != "" {
			return reason, err
		}
	}
	if query.Ecosystem == models.EcosystemPub {
		if hosted := os.Getenv("PUB_HOSTED_URL"); hosted != "" && !publicSource(hosted, "pub.dev", "/") {
			return "PUB_HOSTED_URL selects an unsupported source; public requests were skipped. Assessment is unavailable.", nil
		}
	}
	if query.Ecosystem == models.EcosystemHex && strings.Contains(query.Package, "/") {
		return "Repository-qualified Hex package is not a public hexpm identity; public requests were skipped. Assessment is unavailable.", nil
	}
	if query.Ecosystem == models.EcosystemGitHubActions && (strings.HasPrefix(query.Package, "./") || strings.HasPrefix(query.Package, "../") || strings.HasPrefix(query.Package, "/") || strings.HasPrefix(query.Package, "docker://")) {
		return "Local or container action is not a public GitHub repository identity; public requests were skipped. Assessment is unavailable.", nil
	}
	if query.SourceFile != "" {
		if !filepath.IsAbs(query.SourceFile) {
			return "", fmt.Errorf("source_file must be absolute")
		}
		return artifactRestriction(ctx, query)
	}

	if query.BundlerLock != "" {
		if query.Ecosystem != models.EcosystemRuby {
			return "", fmt.Errorf("bundler_lock is supported only for RubyGems")
		}
		return bundlerRestriction(ctx, query)
	}
	if query.Ecosystem == models.EcosystemGo {
		patterns, err := goPrivatePatterns(ctx)
		if err != nil {
			return "", err
		}
		if isPrivateGoModule(query, patterns) {
			return "Private Go module matches GOPRIVATE; public registry and advisory requests were skipped. Advisory coverage is unavailable.", nil
		}
	}
	if query.ProjectDir != "" {
		if !filepath.IsAbs(query.ProjectDir) {
			return "", fmt.Errorf("project_dir must be absolute")
		}
		switch query.Ecosystem {
		case models.EcosystemMaven:
			return jvmProjectRestriction(ctx, query)
		case models.EcosystemNuGet:
			return nugetProjectRestriction(ctx, query)
		case models.EcosystemNPM:
			registry, err := npmRegistry(ctx, query)
			if err != nil {
				return "", err
			}
			if !publicSource(registry, "registry.npmjs.org", "/") {
				return "Configured npm registry is not the supported public registry; public registry and advisory requests were skipped. Assessment is unavailable.", nil
			}
		case models.EcosystemCargo:
			return cargoRestriction(ctx, query)
		case models.EcosystemPyPI:
			return poetryRestriction(ctx, query)
		default:
			return "", fmt.Errorf("project_dir is supported for npm, Cargo, Poetry, NuGet, Maven, and Gradle")
		}
	}
	if query.NuGetConfig != "" {
		if query.Ecosystem != models.EcosystemNuGet {
			return "", fmt.Errorf("nuget_config is supported only for NuGet")
		}
		return nugetRestriction(query)
	}
	return "", nil
}

func npmRegistry(ctx context.Context, query models.Query) (string, error) {
	return npmRegistryWithConfig(ctx, query, "")
}

func npmRegistryWithConfig(ctx context.Context, query models.Query, config string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	get := func(key string) (string, error) {
		cmd := exec.CommandContext(ctx, "npm", "config", "get", key)
		cmd.Dir = query.ProjectDir
		if config != "" {
			cmd.Env = append(os.Environ(), "NPM_CONFIG_USERCONFIG="+config, "npm_config_userconfig="+config)
		}
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("cannot read npm registry configuration for project")
		}
		return strings.TrimSpace(string(out)), nil
	}
	if strings.HasPrefix(query.Package, "@") {
		scope, _, ok := strings.Cut(query.Package, "/")
		if ok {
			registry, err := get(scope + ":registry")
			if err != nil {
				return "", err
			}
			if registry != "undefined" && registry != "null" && registry != "" {
				return registry, nil
			}
		}
	}
	return get("registry")
}

func publicSource(raw, host, sourcePath string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && strings.EqualFold(u.Host, host) && u.User == nil && u.RawQuery == "" && u.Fragment == "" && strings.TrimRight(u.Path, "/") == strings.TrimRight(sourcePath, "/")
}

func nugetRestriction(query models.Query) (string, error) {
	if !filepath.IsAbs(query.NuGetConfig) {
		return "", fmt.Errorf("nuget_config must be absolute")
	}
	data, err := os.ReadFile(query.NuGetConfig)
	if err != nil {
		return "", fmt.Errorf("cannot read selected NuGet configuration")
	}
	sources, mappings, err := parseNuGetSources(data)
	if err != nil {
		return "", err
	}
	return nugetSourceRestriction(query.Package, sources, mappings)
}

func nugetSourceRestriction(packageName string, sources map[string]string, mappings []nugetSourceMapping) (string, error) {
	selected := map[string]bool{}
	best := -1
	name := strings.ToLower(packageName)
	for _, mapping := range mappings {
		for _, entry := range mapping.Packages {
			pattern := strings.ToLower(entry.Pattern)
			if pattern == "" || strings.Count(pattern, "*") > 1 || (strings.Contains(pattern, "*") && !strings.HasSuffix(pattern, "*")) {
				return "", fmt.Errorf("invalid NuGet package source pattern")
			}
			score := -1
			switch {
			case pattern == "*":
				score = 0
			case !strings.Contains(pattern, "*") && pattern == name:
				score = len(name) + 1
			case strings.HasSuffix(pattern, "*") && strings.Count(pattern, "*") == 1 && strings.HasPrefix(name, strings.TrimSuffix(pattern, "*")):
				score = len(pattern) - 1
			}
			if score < 0 {
				continue
			}
			if score > best {
				selected = map[string]bool{}
				best = score
			}
			if score == best {
				selected[mapping.Key] = true
			}
		}
	}
	if len(mappings) == 0 {
		for key := range sources {
			selected[key] = true
		}
	}
	const reason = "Selected NuGet configuration does not exclusively select the supported public registry for this package; public registry and advisory requests were skipped. Assessment is unavailable."
	if len(selected) == 0 {
		return reason, nil
	}
	for key := range selected {
		if !publicSource(sources[key], "api.nuget.org", "/v3/index.json") {
			return reason, nil
		}
	}
	return "", nil
}
