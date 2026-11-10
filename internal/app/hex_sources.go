package app

import "github.com/clidey/deptrust/internal/models"

func hexArtifactRestriction(q models.Query) (string, error) {
	metadata, err := readArtifactJSON(q.SourceFile)
	if err != nil {
		return "", err
	}
	if text(metadata["name"]) != q.Package || text(metadata["repository"]) != "hexpm" || !publicSource(text(metadata["html_url"]), "hex.pm", "/packages/"+q.Package) {
		return unsupportedArtifactSource, nil
	}
	return "", nil
}
