package registry

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/clidey/deptrust/internal/models"
)

type hexMetadata struct {
	LatestVersion       string `json:"latest_version"`
	LatestStableVersion string `json:"latest_stable_version"`
	Releases            []struct {
		Version    string `json:"version"`
		InsertedAt string `json:"inserted_at"`
	} `json:"releases"`
}

func resolveHex(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	endpoint := "https://hex.pm/api/packages/" + url.PathEscape(query.Package)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return VersionInfo{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("fetch Hex.pm metadata: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		if err := resp.Body.Close(); err != nil {
			return VersionInfo{}, fmt.Errorf("close Hex.pm metadata response: %w", err)
		}
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	var metadata hexMetadata
	if err := decodeJSON(resp, &metadata); err != nil {
		return VersionInfo{}, err
	}

	versionSet := map[string]struct{}{}
	publishedAtByVersion := map[string]*time.Time{}
	for _, version := range metadata.Releases {
		versionSet[version.Version] = struct{}{}
		publishedAtByVersion[version.Version] = parseTime(version.InsertedAt)
	}
	latest := metadata.LatestStableVersion
	if latest == "" {
		latest = metadata.LatestVersion
	}
	if latest == "" && len(versionSet) > 0 {
		latest = sortedVersionKeys(versionSet)[0]
	}
	requested := canonicalRequestedVersion(query.Version, latest, versionSet)
	if requested == "" {
		return VersionInfo{}, fmt.Errorf("hex.pm package %q does not declare a latest version", query.Package)
	}
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
