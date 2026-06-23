package registry

import (
	"net/url"
	"strings"

	"github.com/clidey/deptrust/internal/models"
)

func canonicalRequestedVersion(requested, latest string, versionSet map[string]struct{}) string {
	requested = strings.TrimSpace(requested)
	if requested == "" || strings.EqualFold(requested, models.LatestVersion) {
		return latest
	}
	if _, ok := versionSet[requested]; ok {
		return requested
	}
	for version := range versionSet {
		if strings.EqualFold(version, requested) {
			return version
		}
	}
	return requested
}

func pathEscapeSegments(value string) string {
	parts := strings.Split(value, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func CompareVersionsForApp(left, right string) int {
	return compareVersion(left, right)
}
