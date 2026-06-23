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

type rubyGemVersion struct {
	Number    string `json:"number"`
	CreatedAt string `json:"created_at"`
	BuiltAt   string `json:"built_at"`
}

type rubyGemLatest struct {
	Version string `json:"version"`
}

func resolveRubyGems(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	escapedName := url.PathEscape(query.Package)
	versionsEndpoint := "https://rubygems.org/api/v1/versions/" + escapedName + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionsEndpoint, nil)
	if err != nil {
		return VersionInfo{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("fetch RubyGems versions: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		if err := resp.Body.Close(); err != nil {
			return VersionInfo{}, fmt.Errorf("close RubyGems versions response: %w", err)
		}
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	var metadata []rubyGemVersion
	if err := decodeJSON(resp, &metadata); err != nil {
		return VersionInfo{}, err
	}

	latest := rubyLatestVersion(ctx, client, escapedName)
	versionSet := map[string]struct{}{}
	publishedAtByVersion := map[string]*time.Time{}
	for _, version := range metadata {
		if version.Number == "" {
			continue
		}
		versionSet[version.Number] = struct{}{}
		if latest == "" {
			latest = version.Number
		}
		publishedAtByVersion[version.Number] = parseTime(firstNonEmpty(version.CreatedAt, version.BuiltAt))
	}

	requested := strings.TrimSpace(query.Version)
	if requested == "" || strings.EqualFold(requested, models.LatestVersion) {
		requested = latest
	}
	if requested == "" {
		return VersionInfo{}, fmt.Errorf("RubyGems package %q does not declare a latest version", query.Package)
	}
	if _, ok := versionSet[requested]; !ok {
		return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: requested, Latest: latest}
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

func rubyLatestVersion(ctx context.Context, client HTTPClient, escapedName string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://rubygems.org/api/v1/versions/"+escapedName+"/latest.json", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}

	var latest rubyGemLatest
	if err := decodeJSON(resp, &latest); err != nil {
		return ""
	}
	return latest.Version
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
