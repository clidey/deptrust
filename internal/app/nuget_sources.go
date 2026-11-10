package app

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

type nugetSourceMapping struct {
	Key      string `xml:"key,attr"`
	Packages []struct {
		Pattern string `xml:"pattern,attr"`
	} `xml:"package"`
}

// parseNuGetSources applies ordered collection operations in a selected config.
// It deliberately does not reconstruct machine/user/directory inheritance.
func parseNuGetSources(data []byte) (map[string]string, []nugetSourceMapping, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	sources := map[string]string{}
	disabled := map[string]string{}
	mappings := map[string]nugetSourceMapping{}
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
				if err := decoder.DecodeElement(&mapping, &element); err != nil || mapping.Key == "" {
					return nil, nil, fmt.Errorf("invalid NuGet source mapping")
				}
				// Source keys are unique in the selected configuration.
				if _, exists := mappings[mapping.Key]; exists {
					return nil, nil, fmt.Errorf("duplicate NuGet source mapping")
				}
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
					sources[attrs["key"]] = attrs["value"]
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
