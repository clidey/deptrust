package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/clidey/deptrust/internal/models"
	"github.com/pelletier/go-toml/v2"
)

var pythonNameSeparators = regexp.MustCompile(`[-_.]+`)
var requirementName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*`)

func pythonName(name string) string {
	return strings.ToLower(pythonNameSeparators.ReplaceAllString(name, "-"))
}

func readProjectTOML(dir, file string) (map[string]any, error) {
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return nil, fmt.Errorf("cannot read selected project TOML")
	}
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("invalid project TOML")
	}
	return root, nil
}
func table(value any) map[string]any { result, _ := value.(map[string]any); return result }
func array(value any) []any          { result, _ := value.([]any); return result }
func text(value any) string          { result, _ := value.(string); return result }

func poetryRestriction(_ context.Context, query models.Query) (string, error) {
	root, err := readProjectTOML(query.ProjectDir, "pyproject.toml")
	if err != nil {
		return "", err
	}
	poetry := table(table(root["tool"])["poetry"])
	sources := map[string]string{}
	for _, raw := range array(poetry["source"]) {
		source := table(raw)
		name := strings.ToLower(text(source["name"]))
		priority := text(source["priority"])
		if priority == "" {
			priority = "primary"
		}
		if name == "" || (priority != "primary" && priority != "supplemental" && priority != "explicit") || source["default"] != nil || source["secondary"] != nil {
			return "", fmt.Errorf("unsupported Poetry source declaration")
		}
		if _, exists := sources[name]; exists {
			return "", fmt.Errorf("duplicate Poetry source")
		}
		if name == "pypi" && source["url"] != nil {
			return "", fmt.Errorf("PyPI cannot have a source URL override")
		}
		sources[name] = priority
	}
	entries := []any{}
	collect := func(deps map[string]any) {
		for name, value := range deps {
			if pythonName(name) == pythonName(query.Package) {
				if list, ok := value.([]any); ok {
					entries = append(entries, list...)
				} else {
					entries = append(entries, value)
				}
			}
		}
	}
	collect(table(poetry["dependencies"]))
	collect(table(poetry["dev-dependencies"]))
	for _, group := range table(poetry["group"]) {
		collect(table(table(group)["dependencies"]))
	}
	// Poetry tables enrich all matching PEP 621/group requirements; direct URLs
	// still take precedence because their artifact is not a PyPI identity.
	collectRequirements := func(list []any) {
		for _, raw := range list {
			requirement, ok := raw.(string)
			if !ok {
				continue
			}
			if pythonName(requirementName.FindString(requirement)) != pythonName(query.Package) {
				continue
			}
			if strings.Contains(requirement, "@") {
				entries = append(entries, map[string]any{"url": true})
			} else if len(entries) == 0 {
				entries = append(entries, requirement)
			}
		}
	}
	project := table(root["project"])
	collectRequirements(array(project["dependencies"]))
	for _, list := range table(project["optional-dependencies"]) {
		collectRequirements(array(list))
	}
	// All groups are considered, so include-group expansion cannot hide a source.
	for _, list := range table(root["dependency-groups"]) {
		collectRequirements(array(list))
	}
	restricted := len(entries) == 0
	for _, entry := range entries {
		switch dep := entry.(type) {
		case map[string]any:
			if dep["git"] != nil || dep["path"] != nil || dep["url"] != nil {
				restricted = true
				continue
			}
			if value, exists := dep["source"]; exists {
				source := strings.ToLower(text(value))
				if source == "" {
					return "", fmt.Errorf("invalid Poetry source constraint")
				}
				if source != "pypi" {
					if _, exists := sources[source]; !exists {
						return "", fmt.Errorf("undefined Poetry source")
					}
					restricted = true
				}
				continue
			}
		case string:
		default:
			return "", fmt.Errorf("unsupported Poetry dependency declaration")
		}
		for name, priority := range sources {
			if name != "pypi" && priority != "explicit" {
				restricted = true
			}
		}
	}
	if restricted {
		return "Poetry project does not exclusively declare this package from PyPI; public registry and advisory requests were skipped. Assessment is unavailable.", nil
	}
	return "", nil
}
