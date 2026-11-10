package app

import "github.com/clidey/deptrust/internal/models"

// Cabal's plan.json writer records the actual package location and repository URI.
func cabalArtifactRestriction(q models.Query) (string, error) {
	root, err := readArtifactJSON(q.SourceFile)
	if err != nil {
		return "", err
	}
	found := false
	for _, raw := range array(root["install-plan"]) {
		pkg := table(raw)
		if text(pkg["pkg-name"]) != q.Package {
			continue
		}
		found = true
		source := table(pkg["pkg-src"])
		repo := table(source["repo"])
		if text(source["type"]) != "repo-tar" || (text(repo["type"]) != "secure-repo" && text(repo["type"]) != "remote-repo") || !publicSource(text(repo["uri"]), "hackage.haskell.org", "/") {
			return unsupportedArtifactSource, nil
		}
	}
	if !found {
		return unsupportedArtifactSource, nil
	}
	return "", nil
}
