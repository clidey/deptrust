package app

import (
	"fmt"
	"os"

	"github.com/clidey/deptrust/internal/models"
	"go.yaml.in/yaml/v3"
)

func readArtifactYAML(file string) (map[string]any, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("cannot read selected source YAML")
	}
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("invalid source YAML")
	}
	if root == nil {
		return nil, fmt.Errorf("empty source YAML")
	}
	return root, nil
}
func dartRestriction(query models.Query) (string, error) {
	root, err := readArtifactYAML(query.SourceFile)
	if err != nil {
		return "", err
	}
	if packages, exists := root["packages"]; exists {
		pkg := table(table(packages)[query.Package])
		if text(pkg["source"]) != "hosted" {
			return unsupportedArtifactSource, nil
		}
		description := table(pkg["description"])
		if text(description["name"]) != query.Package || !publicSource(text(description["url"]), "pub.dev", "/") {
			return unsupportedArtifactSource, nil
		}
		return "", nil
	}
	if root["workspace"] != nil || root["resolution"] == "workspace" {
		return unsupportedArtifactSource, nil
	}
	var entries []any
	for _, section := range []string{"dependencies", "dev_dependencies"} {
		if dep, exists := table(root[section])[query.Package]; exists {
			entries = append(entries, dep)
		}
	}
	if override, exists := table(root["dependency_overrides"])[query.Package]; exists {
		entries = []any{override}
	}
	if len(entries) == 0 {
		return unsupportedArtifactSource, nil
	}
	for _, entry := range entries {
		switch dep := entry.(type) {
		case string:
			continue
		case map[string]any:
			if dep["git"] != nil || dep["path"] != nil || dep["sdk"] != nil {
				return unsupportedArtifactSource, nil
			}
			if hosted, exists := dep["hosted"]; exists {
				url := text(hosted)
				if options := table(hosted); options != nil {
					url = text(options["url"])
					if name := text(options["name"]); name != "" && name != query.Package {
						return unsupportedArtifactSource, nil
					}
				}
				if !publicSource(url, "pub.dev", "/") {
					return unsupportedArtifactSource, nil
				}
			}
		default:
			return "", fmt.Errorf("unsupported Dart dependency declaration")
		}
	}
	return "", nil
}
