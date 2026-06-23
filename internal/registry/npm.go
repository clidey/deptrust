package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/clidey/deptrust/internal/models"
)

type npmMetadata struct {
	DistTags map[string]string          `json:"dist-tags"`
	Versions map[string]json.RawMessage `json:"versions"`
	Time     map[string]string          `json:"time"`
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
	publishedAtByVersion := map[string]*time.Time{}
	for version := range metadata.Versions {
		publishedAtByVersion[version] = parseTime(metadata.Time[version])
	}

	return VersionInfo{
		Ecosystem:            query.Ecosystem,
		Package:              query.Package,
		Version:              requested,
		Latest:               latest,
		Versions:             sortedVersionKeys(metadata.Versions),
		PublishedAt:          publishedAtByVersion[requested],
		PublishedAtByVersion: publishedAtByVersion,
	}, nil
}

func sortedVersionKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return compareVersion(keys[i], keys[j]) > 0
	})
	return keys
}

func parseTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func compareVersion(left, right string) int {
	leftParts := splitVersion(left)
	rightParts := splitVersion(right)
	maxLen := len(leftParts)
	if len(rightParts) > maxLen {
		maxLen = len(rightParts)
	}
	for i := 0; i < maxLen; i++ {
		leftPart := 0
		rightPart := 0
		if i < len(leftParts) {
			leftPart = leftParts[i]
		}
		if i < len(rightParts) {
			rightPart = rightParts[i]
		}
		if leftPart > rightPart {
			return 1
		}
		if leftPart < rightPart {
			return -1
		}
	}
	leftPrerelease := hasPrerelease(left)
	rightPrerelease := hasPrerelease(right)
	if leftPrerelease && !rightPrerelease {
		return -1
	}
	if !leftPrerelease && rightPrerelease {
		return 1
	}
	return strings.Compare(left, right)
}

func hasPrerelease(version string) bool {
	withoutBuild := strings.SplitN(version, "+", 2)[0]
	return strings.Contains(withoutBuild, "-")
}

func splitVersion(version string) []int {
	clean := strings.TrimPrefix(version, "v")
	fields := strings.FieldsFunc(clean, func(r rune) bool {
		return r == '.' || r == '-' || r == '+'
	})
	out := make([]int, 0, len(fields))
	for _, field := range fields {
		value := 0
		for _, r := range field {
			if r < '0' || r > '9' {
				break
			}
			value = value*10 + int(r-'0')
		}
		out = append(out, value)
	}
	return out
}
