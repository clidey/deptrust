package registry

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/clidey/deptrust/internal/models"
)

type nugetVersionsMetadata struct {
	Versions []string `json:"versions"`
}

func resolveNuGet(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	lowerName := strings.ToLower(query.Package)
	endpoint := "https://api.nuget.org/v3-flatcontainer/" + url.PathEscape(lowerName) + "/index.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return VersionInfo{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("fetch NuGet versions: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		if err := resp.Body.Close(); err != nil {
			return VersionInfo{}, fmt.Errorf("close NuGet versions response: %w", err)
		}
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	var metadata nugetVersionsMetadata
	if err := decodeJSON(resp, &metadata); err != nil {
		return VersionInfo{}, err
	}

	versionSet := map[string]struct{}{}
	for _, version := range metadata.Versions {
		versionSet[version] = struct{}{}
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
	requested = canonicalVersion(requested, versionSet)
	if requested == "" {
		return VersionInfo{}, fmt.Errorf("NuGet package %q does not declare a latest version", query.Package)
	}
	if _, ok := versionSet[requested]; !ok {
		return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: query.Version, Latest: latest}
	}

	return VersionInfo{
		Ecosystem: query.Ecosystem,
		Package:   query.Package,
		Version:   requested,
		Latest:    latest,
		Versions:  versions,
	}, nil
}

func canonicalVersion(version string, versionSet map[string]struct{}) string {
	for existing := range versionSet {
		if strings.EqualFold(existing, version) {
			return existing
		}
	}
	return version
}
