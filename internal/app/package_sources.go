package app

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
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
			return "", fmt.Errorf("project_dir is supported for npm, Cargo, and Poetry; use nuget_config for NuGet")
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
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	get := func(key string) (string, error) {
		cmd := exec.CommandContext(ctx, "npm", "config", "get", key)
		cmd.Dir = query.ProjectDir
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

type nugetEntry struct {
	Key   string `xml:"key,attr"`
	Value string `xml:"value,attr"`
}
type nugetSourceMapping struct {
	Key      string `xml:"key,attr"`
	Packages []struct {
		Pattern string `xml:"pattern,attr"`
	} `xml:"package"`
}
type nugetConfiguration struct {
	XMLName  xml.Name             `xml:"configuration"`
	Sources  []nugetEntry         `xml:"packageSources>add"`
	Disabled []nugetEntry         `xml:"disabledPackageSources>add"`
	Mappings []nugetSourceMapping `xml:"packageSourceMapping>packageSource"`
}

func nugetRestriction(query models.Query) (string, error) {
	if !filepath.IsAbs(query.NuGetConfig) {
		return "", fmt.Errorf("nuget_config must be absolute")
	}
	data, err := os.ReadFile(query.NuGetConfig)
	if err != nil {
		return "", fmt.Errorf("cannot read selected NuGet configuration")
	}
	if err := validateNuGetDirectives(data); err != nil {
		return "", err
	}
	var config nugetConfiguration
	if err := xml.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("invalid NuGet configuration")
	}
	sources := map[string]string{}
	for _, entry := range config.Sources {
		sources[entry.Key] = entry.Value
	}
	for _, entry := range config.Disabled {
		if strings.EqualFold(entry.Value, "true") {
			delete(sources, entry.Key)
		}
	}
	selected := map[string]bool{}
	best := -1
	name := strings.ToLower(query.Package)
	for _, mapping := range config.Mappings {
		if _, enabled := sources[mapping.Key]; !enabled {
			continue
		}
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
	if len(config.Mappings) == 0 {
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

// validateNuGetDirectives rejects configuration operations we do not interpret.
func validateNuGetDirectives(data []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	section := ""
	sections := map[string]bool{}
	seenEntry := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("invalid NuGet configuration")
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "packageSources", "disabledPackageSources", "packageSourceMapping":
				if sections[element.Name.Local] {
					return fmt.Errorf("duplicate NuGet configuration section")
				}
				sections[element.Name.Local] = true
				section = element.Name.Local
				seenEntry = false
			case "remove":
				if section != "" {
					return fmt.Errorf("NuGet remove directives are not supported; select a consolidated configuration")
				}
			case "clear":
				if section != "" && seenEntry {
					return fmt.Errorf("NuGet clear directives after entries are not supported; select a consolidated configuration")
				}
			case "add", "packageSource":
				if section != "" {
					seenEntry = true
				}
			}
		case xml.EndElement:
			if element.Name.Local == section {
				section = ""
			}
		}
	}
}
