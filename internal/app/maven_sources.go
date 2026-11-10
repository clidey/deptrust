package app

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/clidey/deptrust/internal/models"
)

// Read an explicitly exported effective POM/settings artifact, not executable
// Maven goals, plugins, or a guessed reconstruction of profile inheritance.
func mavenArtifactRestriction(q models.Query) (string, error) {
	data, err := os.ReadFile(q.SourceFile)
	if err != nil {
		return "", fmt.Errorf("cannot read selected Maven source artifact")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	root := ""
	stack := []string{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("invalid Maven source XML")
		}
		switch element := token.(type) {
		case xml.StartElement:
			if root == "" {
				root = element.Name.Local
				if root != "project" && root != "projects" && root != "settings" {
					return "", fmt.Errorf("select a Maven effective POM or settings artifact")
				}
			}
			stack = append(stack, element.Name.Local)
			if element.Name.Local == "url" && len(stack) > 1 {
				parent := stack[len(stack)-2]
				if parent == "repository" || parent == "mirror" {
					var url string
					if err := decoder.DecodeElement(&url, &element); err != nil {
						return "", fmt.Errorf("invalid Maven source URL")
					}
					stack = stack[:len(stack)-1]
					if !publicSource(strings.TrimSpace(url), "repo.maven.apache.org", "/maven2") && !publicSource(strings.TrimSpace(url), "repo1.maven.org", "/maven2") {
						return unsupportedArtifactSource, nil
					}
				}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if root == "" {
		return "", fmt.Errorf("empty Maven source XML")
	}
	// Settings alone cannot establish POM repositories; POM alone cannot establish
	// mirror/settings overrides. It is a source restriction signal, not provenance.
	return unsupportedArtifactSource, nil
}
