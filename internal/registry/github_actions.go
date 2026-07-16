package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/clidey/deptrust/internal/models"
)

type githubTag struct {
	Name string `json:"name"`
}

func resolveGitHubActions(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	owner, repo, err := splitGitHubRepo(query.Package)
	if err != nil {
		return VersionInfo{}, err
	}

	requested := strings.TrimSpace(query.Version)
	if requested != "" && !strings.EqualFold(requested, models.LatestVersion) {
		exists, err := githubArchiveRefExists(ctx, client, owner, repo, requested)
		if err != nil {
			return VersionInfo{}, err
		}
		if !exists {
			return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: requested}
		}
		return VersionInfo{
			Ecosystem: query.Ecosystem,
			Package:   query.Package,
			Version:   requested,
		}, nil
	}

	tags, err := fetchGitHubActionTags(ctx, client, owner, repo, query.Package)
	if err != nil {
		return VersionInfo{}, err
	}
	versionSet := map[string]struct{}{}
	for _, tag := range tags {
		if tag.Name != "" {
			versionSet[tag.Name] = struct{}{}
		}
	}
	versions := sortedVersionKeys(versionSet)
	if len(versions) == 0 {
		return VersionInfo{}, fmt.Errorf("GitHub action %q does not declare any tags", query.Package)
	}

	return VersionInfo{
		Ecosystem: query.Ecosystem,
		Package:   query.Package,
		Version:   versions[0],
		Latest:    versions[0],
		Versions:  versions,
	}, nil
}

func fetchGitHubActionTags(ctx context.Context, client HTTPClient, owner, repo, packageName string) ([]githubTag, error) {
	endpoint := "https://api.github.com/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/tags?per_page=100"
	var tags []githubTag
	for endpoint != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		setGitHubHeaders(req)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch GitHub action tags: %w", err)
		}
		next := githubNextLink(resp.Header.Get("Link"))
		if resp.StatusCode == http.StatusNotFound {
			if err := resp.Body.Close(); err != nil {
				return nil, fmt.Errorf("close GitHub action tags response: %w", err)
			}
			return nil, PackageNotFoundError{Package: packageName}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			if err := resp.Body.Close(); err != nil {
				return nil, fmt.Errorf("close GitHub action tags response: %w", err)
			}
			return nil, fmt.Errorf("GitHub action tags returned HTTP %d", resp.StatusCode)
		}
		var page []githubTag
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				return nil, fmt.Errorf("decode GitHub action tags: %w (close response: %v)", err, closeErr)
			}
			return nil, fmt.Errorf("decode GitHub action tags: %w", err)
		}
		if err := resp.Body.Close(); err != nil {
			return nil, fmt.Errorf("close GitHub action tags response: %w", err)
		}
		tags = append(tags, page...)
		endpoint = next
	}
	return tags, nil
}

func githubArchiveRefExists(ctx context.Context, client HTTPClient, owner, repo, ref string) (bool, error) {
	endpoint := "https://codeload.github.com/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/tar.gz/" + pathEscapeSegments(ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	req.Method = http.MethodHead
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("verify GitHub action ref: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnprocessableEntity {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("GitHub action ref verification returned HTTP %d", resp.StatusCode)
	}
	return true, nil
}

func setGitHubHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "deptrust")
}

func githubNextLink(header string) string {
	for _, item := range strings.Split(header, ",") {
		parts := strings.Split(item, ";")
		if len(parts) < 2 || !strings.Contains(parts[1], `rel="next"`) {
			continue
		}
		candidate := strings.TrimSpace(parts[0])
		if strings.HasPrefix(candidate, "<https://api.github.com/") && strings.HasSuffix(candidate, ">") {
			return strings.TrimSuffix(strings.TrimPrefix(candidate, "<"), ">")
		}
	}
	return ""
}

func splitGitHubRepo(pkg string) (string, string, error) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(pkg), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("GitHub Actions package must be owner/repo, got %q", pkg)
	}
	return parts[0], parts[1], nil
}
