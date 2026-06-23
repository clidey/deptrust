package registry

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/clidey/deptrust/internal/models"
)

type cargoVersionsMetadata struct {
	Versions []struct {
		Num    string `json:"num"`
		Yanked bool   `json:"yanked"`
	} `json:"versions"`
}

func resolveCargo(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	endpoint := "https://crates.io/api/v1/crates/" + url.PathEscape(query.Package) + "/versions"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return VersionInfo{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("fetch crates.io metadata: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		if err := resp.Body.Close(); err != nil {
			return VersionInfo{}, fmt.Errorf("close crates.io metadata response: %w", err)
		}
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	var metadata cargoVersionsMetadata
	if err := decodeJSON(resp, &metadata); err != nil {
		return VersionInfo{}, err
	}

	versions := make([]string, 0, len(metadata.Versions))
	latest := ""
	versionSet := map[string]struct{}{}
	for _, version := range metadata.Versions {
		if version.Yanked {
			continue
		}
		versions = append(versions, version.Num)
		versionSet[version.Num] = struct{}{}
		if latest == "" {
			latest = version.Num
		}
	}

	requested := strings.TrimSpace(query.Version)
	if requested == "" || strings.EqualFold(requested, models.LatestVersion) {
		requested = latest
	}
	if requested == "" {
		return VersionInfo{}, fmt.Errorf("crate %q does not declare a latest non-yanked version", query.Package)
	}
	if _, ok := versionSet[requested]; !ok {
		return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: requested, Latest: latest}
	}

	return VersionInfo{
		Ecosystem: query.Ecosystem,
		Package:   query.Package,
		Version:   requested,
		Latest:    latest,
		Versions:  versions,
	}, nil
}
