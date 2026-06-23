package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/clidey/deptrust/internal/models"
)

type npmMetadata struct {
	DistTags map[string]string          `json:"dist-tags"`
	Versions map[string]json.RawMessage `json:"versions"`
}

func resolveNPM(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	endpoint := "https://registry.npmjs.org/" + url.PathEscape(query.Package)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return VersionInfo{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("fetch npm metadata: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		if err := resp.Body.Close(); err != nil {
			return VersionInfo{}, fmt.Errorf("close npm metadata response: %w", err)
		}
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	var metadata npmMetadata
	if err := decodeJSON(resp, &metadata); err != nil {
		return VersionInfo{}, err
	}

	latest := metadata.DistTags["latest"]
	requested := strings.TrimSpace(query.Version)
	if requested == "" || strings.EqualFold(requested, models.LatestVersion) {
		requested = latest
	}
	if requested == "" {
		return VersionInfo{}, fmt.Errorf("npm package %q does not declare a latest version", query.Package)
	}
	if _, ok := metadata.Versions[requested]; !ok {
		return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: requested, Latest: latest}
	}

	return VersionInfo{
		Ecosystem: query.Ecosystem,
		Package:   query.Package,
		Version:   requested,
		Latest:    latest,
		Versions:  sortedMapKeys(metadata.Versions),
	}, nil
}

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
