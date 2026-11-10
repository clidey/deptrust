package app

import (
	"encoding/xml"
	"fmt"
	"os"

	"github.com/clidey/deptrust/internal/models"
)

// Read an explicitly exported effective POM/settings artifact, not executable
// Maven goals, plugins, or a guessed reconstruction of profile inheritance.
func mavenArtifactRestriction(q models.Query) (string, error) {
	data, err := os.ReadFile(q.SourceFile)
	if err != nil {
		return "", fmt.Errorf("cannot read selected Maven source artifact")
	}
	var artifact struct{ XMLName xml.Name }
	if err := xml.Unmarshal(data, &artifact); err != nil {
		return "", fmt.Errorf("invalid Maven source XML")
	}
	switch artifact.XMLName.Local {
	case "project", "projects", "settings":
		return unsupportedArtifactSource, nil
	default:
		return "", fmt.Errorf("select a Maven effective POM or settings artifact")
	}
}
