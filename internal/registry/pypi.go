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

type pypiMetadata struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
	Releases map[string][]struct {
		UploadTimeISO8601 string `json:"upload_time_iso_8601"`
	} `json:"releases"`
}

func resolvePyPI(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	endpoint := "https://pypi.org/pypi/" + url.PathEscape(query.Package) + "/json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return VersionInfo{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("fetch PyPI metadata: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		if err := resp.Body.Close(); err != nil {
			return VersionInfo{}, fmt.Errorf("close PyPI metadata response: %w", err)
		}
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	var metadata pypiMetadata
	if err := decodeJSON(resp, &metadata); err != nil {
		return VersionInfo{}, err
	}

	latest := metadata.Info.Version
	requested := strings.TrimSpace(query.Version)
	if requested == "" || strings.EqualFold(requested, models.LatestVersion) {
		requested = latest
	}
	if requested == "" {
		return VersionInfo{}, fmt.Errorf("PyPI package %q does not declare a latest version", query.Package)
	}
	if _, ok := metadata.Releases[requested]; !ok {
		return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: requested, Latest: latest}
	}
	publishedAtByVersion := map[string]*time.Time{}
	for version, files := range metadata.Releases {
		publishedAtByVersion[version] = pypiPublishedAt(files)
	}

	return VersionInfo{
		Ecosystem:            query.Ecosystem,
		Package:              query.Package,
		Version:              requested,
		Latest:               latest,
		Versions:             sortedVersionKeys(metadata.Releases),
		PublishedAt:          publishedAtByVersion[requested],
		PublishedAtByVersion: publishedAtByVersion,
	}, nil
}

func pypiPublishedAt(files []struct {
	UploadTimeISO8601 string `json:"upload_time_iso_8601"`
}) *time.Time {
	for _, file := range files {
		if parsed := parseTime(file.UploadTimeISO8601); parsed != nil {
			return parsed
		}
	}
	return nil
}
