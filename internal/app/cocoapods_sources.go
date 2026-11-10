package app

import (
	"github.com/clidey/deptrust/internal/models"
	"strings"
)

func cocoapodsArtifactRestriction(q models.Query) (string, error) {
	root, err := readArtifactYAML(q.SourceFile)
	if err != nil {
		return "", err
	}
	name := strings.Split(q.Package, "/")[0]
	if name != q.Package {
		return unsupportedArtifactSource, nil
	}
	for key := range table(root["EXTERNAL SOURCES"]) {
		if strings.Split(key, "/")[0] == name {
			return unsupportedArtifactSource, nil
		}
	}
	found := false
	for repository, raw := range table(root["SPEC REPOS"]) {
		for _, value := range array(raw) {
			if strings.Split(text(value), "/")[0] != name {
				continue
			}
			found = true
			if repository != "trunk" && !publicSource(repository, "cdn.cocoapods.org", "/") && !publicSource(repository, "github.com", "/CocoaPods/Specs.git") {
				return unsupportedArtifactSource, nil
			}
		}
	}
	if !found {
		return unsupportedArtifactSource, nil
	}
	return "", nil
}
