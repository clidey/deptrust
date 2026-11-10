package app

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"github.com/clidey/deptrust/internal/models"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type nugetSourceMapping struct {
	Unknown  []struct{} `xml:",any"`
	Key      string     `xml:"key,attr"`
	Packages []struct {
		Pattern string `xml:"pattern,attr"`
	} `xml:"package"`
}

// parseNuGetSources applies ordered collection operations in a selected config.
// The same ordered parser is used for explicit and CLI-discovered inherited files.
func parseNuGetSources(data []byte) (map[string]string, []nugetSourceMapping, error) {
	return parseNuGetDocuments([][]byte{data})
}

func parseNuGetDocuments(documents [][]byte) (map[string]string, []nugetSourceMapping, error) {
	sources := map[string]string{}
	disabled := map[string]string{}
	mappings := map[string]nugetSourceMapping{}
	for _, data := range documents {
		decoder := xml.NewDecoder(bytes.NewReader(data))
		localMappings := map[string]bool{}
		sections := map[string]bool{}
		root := false
		section := ""
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, nil, fmt.Errorf("invalid NuGet configuration")
			}
			switch element := token.(type) {
			case xml.StartElement:
				if !root {
					if element.Name.Local != "configuration" {
						return nil, nil, fmt.Errorf("invalid NuGet configuration root")
					}
					root = true
					continue
				}
				if section == "" {
					switch element.Name.Local {
					case "packageSources", "disabledPackageSources", "packageSourceMapping":
						if sections[element.Name.Local] {
							return nil, nil, fmt.Errorf("duplicate NuGet configuration section")
						}
						sections[element.Name.Local] = true
						section = element.Name.Local
					default:
						if err := decoder.Skip(); err != nil {
							return nil, nil, fmt.Errorf("invalid NuGet configuration")
						}
					}
					continue
				}
				attrs := map[string]string{}
				for _, attr := range element.Attr {
					attrs[attr.Name.Local] = attr.Value
				}
				if section == "packageSourceMapping" && element.Name.Local == "packageSource" {
					var mapping nugetSourceMapping
					if err := decoder.DecodeElement(&mapping, &element); err != nil || mapping.Key == "" || len(mapping.Unknown) > 0 {
						return nil, nil, fmt.Errorf("invalid NuGet source mapping")
					}
					// Source keys are unique in the selected configuration.
					if localMappings[mapping.Key] {
						return nil, nil, fmt.Errorf("duplicate NuGet source mapping")
					}
					localMappings[mapping.Key] = true
					mappings[mapping.Key] = mapping
					continue
				}
				switch element.Name.Local {
				case "clear":
					switch section {
					case "packageSources":
						sources = map[string]string{}
					case "disabledPackageSources":
						disabled = map[string]string{}
					case "packageSourceMapping":
						mappings = map[string]nugetSourceMapping{}
					}
				case "remove":
					if attrs["key"] == "" {
						return nil, nil, fmt.Errorf("NuGet remove requires a key")
					}
					switch section {
					case "packageSources":
						delete(sources, attrs["key"])
					case "disabledPackageSources":
						delete(disabled, attrs["key"])
					case "packageSourceMapping":
						return nil, nil, fmt.Errorf("NuGet source mapping removal is unsupported")
					}
				case "add":
					if attrs["key"] == "" {
						return nil, nil, fmt.Errorf("NuGet source entry requires a key")
					}
					switch section {
					case "packageSources":
						sources[attrs["key"]] = expandNuGetEnvironment(attrs["value"])
					case "disabledPackageSources":
						disabled[attrs["key"]] = attrs["value"]
					default:
						return nil, nil, fmt.Errorf("invalid NuGet mapping entry")
					}
				default:
					return nil, nil, fmt.Errorf("unsupported NuGet source directive")
				}
				if err := decoder.Skip(); err != nil {
					return nil, nil, fmt.Errorf("invalid NuGet source directive")
				}
			case xml.EndElement:
				if element.Name.Local == section {
					section = ""
				}
			}
		}
		if !root {
			return nil, nil, fmt.Errorf("empty NuGet configuration")
		}
	}
	for key, value := range disabled {
		if strings.EqualFold(value, "true") {
			delete(sources, key)
		}
	}
	list := make([]nugetSourceMapping, 0, len(mappings))
	for _, mapping := range mappings {
		list = append(list, mapping)
	}
	return sources, list, nil
}

var nugetEnvironment = regexp.MustCompile(`%([^%]+)%`)

func expandNuGetEnvironment(value string) string {
	return nugetEnvironment.ReplaceAllStringFunc(value, func(match string) string {
		if replacement, exists := os.LookupEnv(match[1 : len(match)-1]); exists {
			return replacement
		}
		return match
	})
}

func nugetProjectRestriction(ctx context.Context, q models.Query) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "dotnet", "nuget", "config", "paths", "--working-directory", q.ProjectDir, "--force-english-output")
	cmd.Env = append(os.Environ(), "DOTNET_CLI_TELEMETRY_OPTOUT=1", "DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1", "DOTNET_NOLOGO=1", "DOTNET_GENERATE_ASPNET_CERTIFICATE=false")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot discover effective NuGet configuration; a .NET SDK supporting nuget config paths is required")
	}
	var documents [][]byte
	paths := strings.Split(strings.TrimSpace(string(out)), "\n")
	// The official CLI lists highest-priority files first. Apply lowest first.
	for i := len(paths) - 1; i >= 0; i-- {
		file := strings.TrimSpace(paths[i])
		if file == "" {
			continue
		}
		if !filepath.IsAbs(file) {
			return "", fmt.Errorf("unexpected NuGet configuration path output")
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("cannot read inherited NuGet configuration")
		}
		documents = append(documents, data)
	}
	sources, mappings, err := parseNuGetDocuments(documents)
	if err != nil {
		return "", err
	}
	return nugetSourceRestriction(q.Package, sources, mappings)
}
