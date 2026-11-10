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

func publicPyPI(raw string) bool { return publicSource(raw, "pypi.org", "/simple") }
func pythonEnvironmentRestriction() (string, error) {
	for _, key := range []string{"PIP_INDEX_URL", "PIP_EXTRA_INDEX_URL", "UV_INDEX_URL", "UV_EXTRA_INDEX_URL", "UV_DEFAULT_INDEX", "UV_INDEX"} {
		for _, value := range strings.Fields(os.Getenv(key)) {
			if !publicPyPI(value) {
				return "Python index environment selects an unsupported source; public requests were skipped. Assessment is unavailable.", nil
			}
		}
	}
	for _, key := range []string{"PIP_FIND_LINKS", "PIP_NO_INDEX", "UV_FIND_LINKS", "UV_NO_INDEX"} {
		if value := os.Getenv(key); value != "" && value != "0" && !strings.EqualFold(value, "false") {
			return unsupportedArtifactSource, nil
		}
	}
	if file := os.Getenv("PIP_CONFIG_FILE"); file != "" && file != os.DevNull {
		return pipConfigRestriction(file)
	}
	return "", nil
}
func pythonArtifactRestriction(ctx context.Context, q models.Query) (string, error) {
	switch filepath.Base(q.SourceFile) {
	case "pip.conf", "pip.ini":
		return pipConfigRestriction(q.SourceFile)
	case "Pipfile.lock":
		root, err := readArtifactJSON(q.SourceFile)
		if err != nil {
			return "", err
		}
		return pipenvRestriction(root, q.Package, true)
	case "Pipfile":
		root, err := readProjectTOML(filepath.Dir(q.SourceFile), filepath.Base(q.SourceFile))
		if err != nil {
			return "", err
		}
		return pipenvRestriction(root, q.Package, false)
	case "uv.lock", "pyproject.toml":
		root, err := readProjectTOML(filepath.Dir(q.SourceFile), filepath.Base(q.SourceFile))
		if err != nil {
			return "", err
		}
		if filepath.Base(q.SourceFile) == "uv.lock" {
			return uvLockRestriction(root, q.Package)
		}
		tool := table(root["tool"])
		if tool["uv"] != nil && tool["poetry"] != nil {
			return "", fmt.Errorf("ambiguous Python package manager configuration")
		}
		if tool["uv"] != nil {
			return uvProjectRestriction(root, q.Package)
		}
		q.ProjectDir = filepath.Dir(q.SourceFile)
		return poetryRestriction(ctx, q)
	default:
		return "", fmt.Errorf("unsupported Python source artifact; select pyproject.toml, uv.lock, Pipfile, Pipfile.lock, pip.conf, or pip.ini")
	}
}
func uvLockRestriction(root map[string]any, name string) (string, error) {
	found := false
	for _, raw := range array(root["package"]) {
		pkg := table(raw)
		if pythonName(text(pkg["name"])) != pythonName(name) {
			continue
		}
		found = true
		source := table(pkg["source"])
		if !publicPyPI(text(source["registry"])) {
			return unsupportedArtifactSource, nil
		}
	}
	if !found {
		return unsupportedArtifactSource, nil
	}
	return "", nil
}
func uvProjectRestriction(root map[string]any, name string) (string, error) {
	uv := table(table(root["tool"])["uv"])
	indexes := map[string]string{}
	customDefault := false
	for _, raw := range array(uv["index"]) {
		index := table(raw)
		key := text(index["name"])
		url := text(index["url"])
		if key != "" {
			if _, exists := indexes[key]; exists {
				return "", fmt.Errorf("duplicate uv index")
			}
			indexes[key] = url
		}
		if index["explicit"] != true && !publicPyPI(url) {
			customDefault = true
		}
	}
	for key, raw := range table(uv["sources"]) {
		if pythonName(key) != pythonName(name) {
			continue
		}
		entries := []any{raw}
		if list, ok := raw.([]any); ok {
			entries = list
		}
		for _, entry := range entries {
			source := table(entry)
			index := text(source["index"])
			if index == "" || !publicPyPI(indexes[index]) {
				return unsupportedArtifactSource, nil
			}
		}
		return "", nil
	}
	if customDefault || uv["find-links"] != nil || uv["index-url"] != nil || uv["extra-index-url"] != nil {
		return unsupportedArtifactSource, nil
	}
	project := table(root["project"])
	lists := []any{project["dependencies"]}
	for _, list := range table(project["optional-dependencies"]) {
		lists = append(lists, list)
	}
	for _, list := range table(root["dependency-groups"]) {
		lists = append(lists, list)
	}
	found := false
	for _, raw := range lists {
		for _, value := range array(raw) {
			requirement := text(value)
			if pythonName(requirementName.FindString(requirement)) == pythonName(name) {
				found = true
				if strings.Contains(requirement, "@") {
					return unsupportedArtifactSource, nil
				}
			}
		}
	}
	if !found {
		return unsupportedArtifactSource, nil
	}
	return "", nil
}
func pipenvRestriction(root map[string]any, name string, locked bool) (string, error) {
	rawSources := array(root["source"])
	if locked {
		rawSources = array(table(root["_meta"])["sources"])
	}
	sources := map[string]string{}
	for _, raw := range rawSources {
		source := table(raw)
		key := text(source["name"])
		if key == "" {
			return "", fmt.Errorf("invalid Pipenv source")
		}
		sources[key] = text(source["url"])
	}
	found := false
	for category, raw := range root {
		if category == "_meta" || category == "requires" || category == "pipenv" {
			continue
		}
		for key, value := range table(raw) {
			if pythonName(key) != pythonName(name) {
				continue
			}
			found = true
			dep := table(value)
			if dep["git"] != nil || dep["path"] != nil || dep["file"] != nil {
				return unsupportedArtifactSource, nil
			}
			if index := text(dep["index"]); index != "" {
				if !publicPyPI(sources[index]) {
					return unsupportedArtifactSource, nil
				}
			} else {
				for _, url := range sources {
					if !publicPyPI(url) {
						return unsupportedArtifactSource, nil
					}
				}
			}
		}
	}
	if !found {
		return unsupportedArtifactSource, nil
	}
	return "", nil
}
func pipConfigRestriction(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("cannot read selected pip configuration")
	}
	sections := map[string]map[string]string{}
	section := ""
	last := ""
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			sections[section] = map[string]string{}
			last = ""
			continue
		}
		if section == "" {
			return "", fmt.Errorf("invalid pip configuration")
		}
		if last != "" && len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\t') {
			sections[section][last] += " " + line
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return "", fmt.Errorf("unsupported pip configuration syntax")
		}
		key = strings.ToLower(strings.TrimSpace(key))
		sections[section][key] = strings.TrimSpace(value)
		last = key
	}
	if scanner.Err() != nil {
		return "", fmt.Errorf("cannot parse pip configuration")
	}
	effective := map[string]string{}
	for _, name := range []string{"global", "install"} {
		for key, value := range sections[name] {
			effective[key] = value
		}
	}
	for _, key := range []string{"index-url", "extra-index-url"} {
		for _, url := range strings.Fields(effective[key]) {
			if !publicPyPI(url) {
				return unsupportedArtifactSource, nil
			}
		}
	}
	if effective["find-links"] != "" {
		return unsupportedArtifactSource, nil
	}
	if value := effective["no-index"]; value != "" && value != "0" && !strings.EqualFold(value, "false") {
		return unsupportedArtifactSource, nil
	}
	return "", nil
}
