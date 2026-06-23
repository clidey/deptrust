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

type mavenSearchResponse struct {
	Response struct {
		Docs []struct {
			GroupID    string `json:"g"`
			ArtifactID string `json:"a"`
			Version    string `json:"v"`
			Timestamp  int64  `json:"timestamp"`
		} `json:"docs"`
	} `json:"response"`
}

func resolveMaven(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	groupID, artifactID, err := splitMavenPackage(query.Package)
	if err != nil {
		return VersionInfo{}, err
	}

	params := url.Values{}
	params.Set("q", fmt.Sprintf("g:%s AND a:%s", groupID, artifactID))
	params.Set("core", "gav")
	params.Set("rows", "200")
	params.Set("wt", "json")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://search.maven.org/solrsearch/select?"+params.Encode(), nil)
	if err != nil {
		return VersionInfo{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("fetch Maven versions: %w", err)
	}

	var metadata mavenSearchResponse
	if err := decodeJSON(resp, &metadata); err != nil {
		return VersionInfo{}, err
	}
	if len(metadata.Response.Docs) == 0 {
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	versionSet := map[string]struct{}{}
	publishedAtByVersion := map[string]*time.Time{}
	for _, doc := range metadata.Response.Docs {
		if doc.Version == "" {
			continue
		}
		versionSet[doc.Version] = struct{}{}
		publishedAtByVersion[doc.Version] = mavenPublishedAt(doc.Timestamp)
	}
	versions := sortedVersionKeys(versionSet)
	latest := ""
	if len(versions) > 0 {
		latest = versions[0]
	}

	requested := strings.TrimSpace(query.Version)
	if requested == "" || strings.EqualFold(requested, models.LatestVersion) {
		requested = latest
	}
	if requested == "" {
		return VersionInfo{}, fmt.Errorf("maven package %q does not declare a latest version", query.Package)
	}
	if _, ok := versionSet[requested]; !ok {
		return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: requested, Latest: latest}
	}

	return VersionInfo{
		Ecosystem:            query.Ecosystem,
		Package:              query.Package,
		Version:              requested,
		Latest:               latest,
		Versions:             versions,
		PublishedAt:          publishedAtByVersion[requested],
		PublishedAtByVersion: publishedAtByVersion,
	}, nil
}

func splitMavenPackage(value string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("maven package must be groupId:artifactId, got %q", value)
	}
	return parts[0], parts[1], nil
}

func mavenPublishedAt(timestampMillis int64) *time.Time {
	if timestampMillis <= 0 {
		return nil
	}
	parsed := time.UnixMilli(timestampMillis).UTC()
	return &parsed
}
