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

type cargoVersionsMetadata struct {
	Versions []struct {
		Num       string `json:"num"`
		Yanked    bool   `json:"yanked"`
		CreatedAt string `json:"created_at"`
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
	allVersionSet := map[string]struct{}{}
	yankedVersions := map[string]struct{}{}
	publishedAtByVersion := map[string]*time.Time{}
	for _, version := range metadata.Versions {
		allVersionSet[version.Num] = struct{}{}
		publishedAtByVersion[version.Num] = parseTime(version.CreatedAt)
		if version.Yanked {
			yankedVersions[version.Num] = struct{}{}
			continue
		}
		versions = append(versions, version.Num)
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
	if _, ok := allVersionSet[requested]; !ok {
		return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: requested, Latest: latest}
	}

	var signals []models.Signal
	if _, yanked := yankedVersions[requested]; yanked {
		signals = []models.Signal{{
			Type:     "yanked_release",
			Severity: "medium",
			Score:    50,
			Message:  fmt.Sprintf("%s %s is yanked on crates.io and should not be selected for a new install.", query.Package, requested),
			Source:   "crates.io",
		}}
	}

	return VersionInfo{
		Ecosystem:            query.Ecosystem,
		Package:              query.Package,
		Version:              requested,
		Latest:               latest,
		Versions:             versions,
		PublishedAt:          publishedAtByVersion[requested],
		PublishedAtByVersion: publishedAtByVersion,
		Signals:              signals,
	}, nil
}
