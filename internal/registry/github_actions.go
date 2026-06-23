package registry

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/clidey/deptrust/internal/models"
)

type githubTag struct {
	Name string `json:"name"`
}

var gitCommitSHARe = regexp.MustCompile(`(?i)^[0-9a-f]{40}$`)

func resolveGitHubActions(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	owner, repo, err := splitGitHubRepo(query.Package)
	if err != nil {
		return VersionInfo{}, err
	}
	endpoint := "https://api.github.com/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/tags?per_page=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return VersionInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("fetch GitHub action tags: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		if err := resp.Body.Close(); err != nil {
			return VersionInfo{}, fmt.Errorf("close GitHub action tags response: %w", err)
		}
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	var tags []githubTag
	if err := decodeJSON(resp, &tags); err != nil {
		return VersionInfo{}, err
	}

	versionSet := map[string]struct{}{}
	for _, tag := range tags {
		if tag.Name != "" {
			versionSet[tag.Name] = struct{}{}
		}
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
	if requested == "" {
		return VersionInfo{}, fmt.Errorf("GitHub action %q does not declare any tags", query.Package)
	}
	if _, ok := versionSet[requested]; !ok && !gitCommitSHARe.MatchString(requested) {
		exists, err := githubBranchRefExists(ctx, client, owner, repo, requested)
		if err != nil {
			return VersionInfo{}, err
		}
		if !exists {
			return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: requested, Latest: latest}
		}
	}

	return VersionInfo{
		Ecosystem: query.Ecosystem,
		Package:   query.Package,
		Version:   requested,
		Latest:    latest,
		Versions:  versions,
	}, nil
}

func githubBranchRefExists(ctx context.Context, client HTTPClient, owner, repo, branch string) (bool, error) {
	endpoint := "https://api.github.com/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/git/ref/heads/" + pathEscapeSegments(branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("fetch GitHub action branch ref: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("GitHub action branch ref returned HTTP %d", resp.StatusCode)
	}
	return true, nil
}

func splitGitHubRepo(pkg string) (string, string, error) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(pkg), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("GitHub Actions package must be owner/repo, got %q", pkg)
	}
	return parts[0], parts[1], nil
}
