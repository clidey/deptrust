package registry

import (
	"context"
	"errors"
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
	requested := strings.TrimSpace(query.Version)
	if requested != "" && !strings.EqualFold(requested, models.LatestVersion) && isPackagistDevVersion(requested) {
		devVersions, err := fetchPackagistVersions(ctx, client, query.Package, true)
		if err != nil {
			var notFound PackageNotFoundError
			if errors.As(err, &notFound) {
				return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: requested}
			}
			return VersionInfo{}, err
		}
		for _, version := range devVersions {
			if strings.EqualFold(version.Version, requested) {
				publishedAt := parseTime(version.Time)
				return VersionInfo{
					Ecosystem:            query.Ecosystem,
					Package:              query.Package,
					Version:              version.Version,
					PublishedAt:          publishedAt,
					PublishedAtByVersion: map[string]*time.Time{version.Version: publishedAt},
				}, nil
			}
		}
		return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: requested}
	}

	versions, err := fetchPackagistVersions(ctx, client, query.Package, false)
	if err != nil {
		return VersionInfo{}, err
	}

	versionSet := map[string]struct{}{}
	publishedAtByVersion := map[string]*time.Time{}
	for _, version := range versions {
		versionSet[version.Version] = struct{}{}
		publishedAtByVersion[version.Version] = parseTime(version.Time)
	}
	latest := versions[0].Version
	requested = canonicalRequestedVersion(query.Version, latest, versionSet)
	if _, ok := versionSet[requested]; !ok {
		return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: strings.TrimSpace(query.Version), Latest: latest}
	}

	return VersionInfo{
		Ecosystem:            query.Ecosystem,
		Package:              query.Package,
		Version:              requested,
		Latest:               latest,
		Versions:             sortedVersionKeys(versionSet),
		PublishedAt:          publishedAtByVersion[requested],
		PublishedAtByVersion: publishedAtByVersion,
	}, nil
}

func fetchPackagistVersions(ctx context.Context, client HTTPClient, packageName string, dev bool) ([]packagistVersion, error) {
	suffix := ""
	if dev {
		suffix = "~dev"
	}
	endpoint := "https://repo.packagist.org/p2/" + pathEscapeSegments(packageName) + suffix + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Packagist metadata: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		if err := resp.Body.Close(); err != nil {
			return nil, fmt.Errorf("close Packagist metadata response: %w", err)
		}
		return nil, PackageNotFoundError{Package: packageName}
	}

	var metadata packagistMetadata
	if err := decodeJSON(resp, &metadata); err != nil {
		return nil, err
	}
	versions := metadata.Packages[packageName]
	if len(versions) == 0 {
		for name, found := range metadata.Packages {
			if strings.EqualFold(name, packageName) {
				versions = found
				break
			}
		}
	}
	if len(versions) == 0 {
		return nil, PackageNotFoundError{Package: packageName}
	}
	return versions, nil
}

func isPackagistDevVersion(version string) bool {
	version = strings.ToLower(strings.TrimSpace(version))
	return strings.HasPrefix(version, "dev-") || strings.HasSuffix(version, "-dev")
}
