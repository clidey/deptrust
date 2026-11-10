package app

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/clidey/deptrust/internal/models"
)

func javascriptArtifactRestriction(ctx context.Context, q models.Query) (string, error) {
	switch filepath.Base(q.SourceFile) {
	case ".npmrc":
		// npm itself applies INI parsing, interpolation, and configuration precedence.
		// The explicitly selected file is used as userconfig, with its directory as project.
		q.ProjectDir = filepath.Dir(q.SourceFile)
		registry, err := npmRegistryWithConfig(ctx, q, q.SourceFile)
		if err != nil {
			return "", err
		}
		if !publicSource(registry, "registry.npmjs.org", "/") {
			return unsupportedArtifactSource, nil
		}
		return "", nil
	case ".yarnrc":
		data, err := os.ReadFile(q.SourceFile)
		if err != nil {
			return "", fmt.Errorf("cannot read Yarn Classic configuration")
		}
		registry := "https://registry.yarnpkg.com"
		scoped := ""
		scope, _, _ := strings.Cut(q.Package, "/")
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			key := strings.Trim(fields[0], `"`)
			if strings.Contains(strings.ToLower(key), "registry") && key != "registry" && !strings.HasSuffix(key, ":registry") {
				return unsupportedArtifactSource, nil
			}
			if key == "registry" || (strings.HasPrefix(scope, "@") && key == scope+":registry") {
				if len(fields) != 2 {
					return "", fmt.Errorf("unsupported Yarn Classic registry declaration")
				}
				if key == "registry" {
					registry = strings.Trim(fields[1], `"`)
				} else {
					scoped = strings.Trim(fields[1], `"`)
				}
			}
		}
		if scanner.Err() != nil {
			return "", fmt.Errorf("invalid Yarn Classic configuration")
		}
		if scoped != "" {
			registry = scoped
		}
		if !publicJSRegistry(registry) {
			return unsupportedArtifactSource, nil
		}
		return "", nil
	case ".yarnrc.yml", "pnpm-workspace.yaml", "config.yaml":
		root, err := readArtifactYAML(q.SourceFile)
		if err != nil {
			return "", err
		}
		registry := "https://registry.npmjs.org"
		if filepath.Base(q.SourceFile) == ".yarnrc.yml" {
			registry = "https://registry.yarnpkg.com"
			if value := text(root["npmRegistryServer"]); value != "" {
				registry = value
			}
			if scope, _, ok := strings.Cut(strings.TrimPrefix(q.Package, "@"), "/"); ok && strings.HasPrefix(q.Package, "@") {
				if value := text(table(table(root["npmScopes"])[scope])["npmRegistryServer"]); value != "" {
					registry = value
				}
			}
		} else {
			for key, value := range table(root["registries"]) {
				if _, ok := value.(string); !ok || (key != "default" && !strings.HasPrefix(key, "@")) {
					return unsupportedArtifactSource, nil
				}
			}
			if value := text(root["registry"]); value != "" {
				registry = value
			}
			if value := text(table(root["registries"])["default"]); value != "" {
				registry = value
			}
			if scope, _, ok := strings.Cut(q.Package, "/"); ok && strings.HasPrefix(scope, "@") {
				if value := text(table(root["registries"])[scope]); value != "" {
					registry = value
				}
			}
			// Named registry routing needs a manager-resolved artifact rather than guesses.
			if root["namedRegistries"] != nil {
				return unsupportedArtifactSource, nil
			}
		}
		if !publicJSRegistry(registry) {
			return unsupportedArtifactSource, nil
		}
		return "", nil
	default:
		return "", fmt.Errorf("unsupported JavaScript source artifact")
	}
}
func publicJSRegistry(url string) bool {
	return publicSource(url, "registry.npmjs.org", "/") || publicSource(url, "registry.yarnpkg.com", "/")
}
