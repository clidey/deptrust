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

type pubMetadata struct {
	Latest struct {
		Version   string `json:"version"`
		Published string `json:"published"`
	} `json:"latest"`
	Versions []struct {
		Version   string `json:"version"`
		Published string `json:"published"`
	} `json:"versions"`
}

func resolvePub(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	endpoint := "https://pub.dev/api/packages/" + url.PathEscape(query.Package)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return VersionInfo{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("fetch pub.dev metadata: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		if err := resp.Body.Close(); err != nil {
			return VersionInfo{}, fmt.Errorf("close pub.dev metadata response: %w", err)
		}
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	var metadata pubMetadata
	if err := decodeJSON(resp, &metadata); err != nil {
		return VersionInfo{}, err
	}

	versionSet := map[string]struct{}{}
	publishedAtByVersion := map[string]*time.Time{}
	for _, version := range metadata.Versions {
		versionSet[version.Version] = struct{}{}
		publishedAtByVersion[version.Version] = parseTime(version.Published)
	}
	latest := metadata.Latest.Version
	if latest == "" && len(metadata.Versions) > 0 {
		latest = sortedVersionKeys(versionSet)[0]
	}
	requested := canonicalRequestedVersion(query.Version, latest, versionSet)
	if requested == "" {
		return VersionInfo{}, fmt.Errorf("pub.dev package %q does not declare a latest version", query.Package)
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
