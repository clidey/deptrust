package app

import (
	"context"
	"github.com/clidey/deptrust/internal/models"
	"path/filepath"
)

func hexArtifactRestriction(ctx context.Context, q models.Query) (string, error) {
	if filepath.Base(q.SourceFile) == "mix.lock" {
		return mixArtifactRestriction(ctx, q)
	}
	metadata, err := readArtifactJSON(q.SourceFile)
	if err != nil {
		return "", err
	}
	if text(metadata["name"]) != q.Package || text(metadata["repository"]) != "hexpm" || !publicSource(text(metadata["html_url"]), "hex.pm", "/packages/"+q.Package) {
		return unsupportedArtifactSource, nil
	}
	return "", nil
}
