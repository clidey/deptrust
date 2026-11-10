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
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			key := strings.Trim(fields[0], `"`)
			if key == "registry" {
				if len(fields) != 2 {
					return "", fmt.Errorf("unsupported Yarn Classic registry declaration")
				}
				registry = strings.Trim(fields[1], `"`)
			}
		}
		if scanner.Err() != nil {
			return "", fmt.Errorf("invalid Yarn Classic configuration")
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
