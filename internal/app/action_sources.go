package app

import (
	"github.com/clidey/deptrust/internal/models"
	"strings"
)

func actionArtifactRestriction(q models.Query) (string, error) {
	metadata, err := readArtifactJSON(q.SourceFile)
	if err != nil {
		return "", err
	}
	private, known := metadata["private"].(bool)
	if !known || private || text(metadata["visibility"]) != "public" || !strings.EqualFold(text(metadata["full_name"]), q.Package) || !publicSource(text(metadata["html_url"]), "github.com", "/"+text(metadata["full_name"])) {
		return unsupportedArtifactSource, nil
	}
	return "", nil
}
