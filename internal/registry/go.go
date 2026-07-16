package registry

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/clidey/deptrust/internal/models"
)

type goVersionInfo struct {
	Version string `json:"Version"`
	Time    string `json:"Time"`
}

func resolveGo(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	escapedModule := escapeGoModulePath(query.Package)
	requested := strings.TrimSpace(query.Version)
	explicitVersion := requested != "" && !strings.EqualFold(requested, models.LatestVersion)
	if explicitVersion {
		requestedInfo, err := fetchGoVersionInfo(ctx, client, escapedModule, requested)
		if err != nil {
			var notFound PackageNotFoundError
			if errors.As(err, &notFound) {
				return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: requested}
			}
			return VersionInfo{}, err
		}
		if requestedInfo.Version == "" {
			return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: requested}
		}
		publishedAt := parseTime(requestedInfo.Time)
		return VersionInfo{
			Ecosystem:            query.Ecosystem,
			Package:              query.Package,
			Version:              requestedInfo.Version,
			PublishedAt:          publishedAt,
			PublishedAtByVersion: map[string]*time.Time{requestedInfo.Version: publishedAt},
		}, nil
	}

	versions, err := fetchGoVersions(ctx, client, escapedModule, query.Package)
	if err != nil {
		return VersionInfo{}, err
	}

	latest := ""
	latestInfo := goVersionInfo{}
	if len(versions) > 0 {
		latest = versions[0]
		latestInfo, err = fetchGoVersionInfo(ctx, client, escapedModule, latest)
	} else {
		latestInfo, err = fetchGoLatest(ctx, client, escapedModule)
		latest = latestInfo.Version
	}
	if err != nil {
		return VersionInfo{}, err
	}

	requested = latest
	if requested == "" {
		return VersionInfo{}, fmt.Errorf("go module %q does not declare a latest version", query.Package)
	}

	publishedAtByVersion := map[string]*time.Time{}
	if latestInfo.Version != "" {
		publishedAtByVersion[latestInfo.Version] = parseTime(latestInfo.Time)
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

func fetchGoVersions(ctx context.Context, client HTTPClient, escapedModule, packageName string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://proxy.golang.org/"+escapedModule+"/@v/list", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Go module versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil, PackageNotFoundError{Package: packageName}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("go module proxy returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Go module versions: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	versionSet := map[string]struct{}{}
	for scanner.Scan() {
		version := strings.TrimSpace(scanner.Text())
		if version != "" {
			versionSet[version] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan Go module versions: %w", err)
	}

	versions := make([]string, 0, len(versionSet))
	for version := range versionSet {
		versions = append(versions, version)
	}
	return sortedVersionKeys(mapFromKeys(versions)), nil
}

func fetchGoVersionInfo(ctx context.Context, client HTTPClient, escapedModule, version string) (goVersionInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://proxy.golang.org/"+escapedModule+"/@v/"+url.PathEscape(version)+".info", nil)
	if err != nil {
		return goVersionInfo{}, err
	}
	return fetchGoInfo(req, client)
}

func fetchGoLatest(ctx context.Context, client HTTPClient, escapedModule string) (goVersionInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://proxy.golang.org/"+escapedModule+"/@latest", nil)
	if err != nil {
		return goVersionInfo{}, err
	}
	return fetchGoInfo(req, client)
}

func fetchGoInfo(req *http.Request, client HTTPClient) (goVersionInfo, error) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return goVersionInfo{}, fmt.Errorf("fetch Go module version info: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		if err := resp.Body.Close(); err != nil {
			return goVersionInfo{}, fmt.Errorf("close Go module version response: %w", err)
		}
		return goVersionInfo{}, PackageNotFoundError{}
	}

	var info goVersionInfo
	if err := decodeJSON(resp, &info); err != nil {
		return goVersionInfo{}, err
	}
	return info, nil
}

func escapeGoModulePath(modulePath string) string {
	segments := strings.Split(modulePath, "/")
	for i, segment := range segments {
		var builder strings.Builder
		for _, r := range segment {
			if unicode.IsUpper(r) {
				builder.WriteByte('!')
				builder.WriteRune(unicode.ToLower(r))
				continue
			}
			builder.WriteRune(r)
		}
		segments[i] = strings.ReplaceAll(url.PathEscape(builder.String()), "%21", "!")
	}
	return strings.Join(segments, "/")
}

func mapFromKeys(keys []string) map[string]struct{} {
	out := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		out[key] = struct{}{}
	}
	return out
}
