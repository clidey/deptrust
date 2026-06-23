package registry

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/clidey/deptrust/internal/models"
)

type packagistMetadata struct {
	Packages map[string][]packagistVersion `json:"packages"`
}

type packagistVersion struct {
	Version string `json:"version"`
	Time    string `json:"time"`
}

func resolvePackagist(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	endpoint := "https://repo.packagist.org/p2/" + pathEscapeSegments(query.Package) + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return VersionInfo{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("fetch Packagist metadata: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		if err := resp.Body.Close(); err != nil {
			return VersionInfo{}, fmt.Errorf("close Packagist metadata response: %w", err)
		}
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	var metadata packagistMetadata
	if err := decodeJSON(resp, &metadata); err != nil {
		return VersionInfo{}, err
	}

	versions := metadata.Packages[query.Package]
	if len(versions) == 0 {
		for name, found := range metadata.Packages {
			if strings.EqualFold(name, query.Package) {
				versions = found
				break
			}
		}
	}
	if len(versions) == 0 {
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	versionSet := map[string]struct{}{}
	publishedAtByVersion := map[string]*time.Time{}
	versionMap := map[string]struct{}{}
	for _, version := range versions {
		versionSet[version.Version] = struct{}{}
		versionMap[version.Version] = struct{}{}
		publishedAtByVersion[version.Version] = parseTime(version.Time)
	}
	latest := versions[0].Version
	requested := canonicalRequestedVersion(query.Version, latest, versionSet)
	if _, ok := versionSet[requested]; !ok {
		return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: strings.TrimSpace(query.Version), Latest: latest}
	}

	return VersionInfo{
		Ecosystem:            query.Ecosystem,
		Package:              query.Package,
		Version:              requested,
		Latest:               latest,
		Versions:             sortedVersionKeys(versionMap),
		PublishedAt:          publishedAtByVersion[requested],
		PublishedAtByVersion: publishedAtByVersion,
	}, nil
}
