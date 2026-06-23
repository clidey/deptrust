package registry

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/clidey/deptrust/internal/models"
)

type hackagePreferred struct {
	NormalVersion     []string `json:"normal-version"`
	DeprecatedVersion []string `json:"deprecated-version"`
}

func resolveHackage(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	endpoint := "https://hackage.haskell.org/package/" + url.PathEscape(query.Package) + "/preferred.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return VersionInfo{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("fetch Hackage metadata: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		if err := resp.Body.Close(); err != nil {
			return VersionInfo{}, fmt.Errorf("close Hackage metadata response: %w", err)
		}
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	var metadata hackagePreferred
	if err := decodeJSON(resp, &metadata); err != nil {
		return VersionInfo{}, err
	}

	versionSet := map[string]struct{}{}
	for _, version := range metadata.NormalVersion {
		versionSet[version] = struct{}{}
	}
	if len(versionSet) == 0 {
		for _, version := range metadata.DeprecatedVersion {
			versionSet[version] = struct{}{}
		}
	}
	if len(versionSet) == 0 {
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}
	versions := sortedVersionKeys(versionSet)
	latest := versions[0]
	requested := canonicalRequestedVersion(query.Version, latest, versionSet)
	if _, ok := versionSet[requested]; !ok {
		return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: strings.TrimSpace(query.Version), Latest: latest}
	}

	return VersionInfo{
		Ecosystem: query.Ecosystem,
		Package:   query.Package,
		Version:   requested,
		Latest:    latest,
		Versions:  versions,
	}, nil
}
