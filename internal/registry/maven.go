package registry

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/clidey/deptrust/internal/models"
)

const mavenCentralBaseURL = "https://repo.maven.apache.org/maven2"

const googleMavenBaseURL = "https://dl.google.com/dl/android/maven2"

type mavenMetadata struct {
	Versioning struct {
		Latest   string   `xml:"latest"`
		Release  string   `xml:"release"`
		Versions []string `xml:"versions>version"`
	} `xml:"versioning"`
}

type mavenSearchResponse struct {
	Response struct {
		Docs []struct {
			GroupID    string `json:"g"`
			ArtifactID string `json:"a"`
			Version    string `json:"v"`
			Timestamp  int64  `json:"timestamp"`
		} `json:"docs"`
	} `json:"response"`
}

func resolveMaven(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	groupID, artifactID, err := splitMavenPackage(query.Package)
	if err != nil {
		return VersionInfo{}, err
	}

	repositories := query.MavenRepositories
	if len(repositories) == 0 {
		repositories = []string{mavenCentralBaseURL, googleMavenBaseURL, PluginPortalURL, IntelliJReleasesURL}
		if artifactID == groupID+".gradle.plugin" {
			repositories = []string{PluginPortalURL, mavenCentralBaseURL, googleMavenBaseURL, IntelliJReleasesURL}
		}
	}
	var metadata mavenMetadata
	var metadataErr error
	var baseURL string
	requested := strings.TrimSpace(query.Version)
	projectPin := len(query.MavenRepositories) > 0 && requested != "" && !strings.EqualFold(requested, models.LatestVersion)
	var firstMetadata *mavenMetadata
	for _, repository := range repositories {
		baseURL = repository
		metadata, metadataErr = fetchMavenMetadata(ctx, client, baseURL, groupID, artifactID, query.Package)
		if metadataErr == nil && projectPin && !slices.Contains(metadata.Versioning.Versions, requested) {
			if firstMetadata == nil {
				first := metadata
				firstMetadata = &first
			}
			continue
		}
		var missing PackageNotFoundError
		if !errors.As(metadataErr, &missing) {
			break
		}
	}

	if firstMetadata != nil && (metadataErr == nil || errors.As(metadataErr, new(PackageNotFoundError))) && !slices.Contains(metadata.Versioning.Versions, requested) {
		metadata = *firstMetadata
		metadataErr = nil
	}
	versionSet := map[string]struct{}{}
	latest := ""
	search := mavenSearchResponse{}
	if metadataErr == nil {
		for _, version := range metadata.Versioning.Versions {
			version = strings.TrimSpace(version)
			if version != "" {
				versionSet[version] = struct{}{}
			}
		}
		latest = mavenMetadataLatest(metadata, versionSet)
	}

	// Maven Search is a fallback for availability, not the authority or part of
	// the successful path. Its index can lag behind Maven Central or time out.
	if len(versionSet) == 0 && len(query.MavenRepositories) == 0 {
		var searchErr error
		search, searchErr = fetchMavenSearch(ctx, client, groupID, artifactID)
		if searchErr == nil {
			baseURL = mavenCentralBaseURL
			for _, doc := range search.Response.Docs {
				if doc.Version != "" {
					versionSet[doc.Version] = struct{}{}
				}
			}
			versions := sortedVersionKeys(versionSet)
			if len(versions) > 0 {
				latest = versions[0]
			}
		}
	}
	if len(versionSet) == 0 {
		if metadataErr != nil {
			return VersionInfo{}, metadataErr
		}
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	versions := sortedVersionKeys(versionSet)
	if latest == "" && len(versions) > 0 {
		latest = versions[0]
	}

	if requested == "" || strings.EqualFold(requested, models.LatestVersion) {
		requested = latest
	}
	if requested == "" {
		return VersionInfo{}, fmt.Errorf("maven package %q does not declare a latest version", query.Package)
	}
	if _, ok := versionSet[requested]; !ok {
		if metadataErr != nil {
			return VersionInfo{}, metadataErr
		}
		return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: requested, Latest: latest}
	}

	publishedAtByVersion := mavenSearchPublishedAt(search, versionSet)
	if publishedAtByVersion[requested] == nil {
		publishedAtByVersion[requested] = fetchMavenPublishedAt(ctx, client, baseURL, groupID, artifactID, requested)
	}

	var implementation *models.PackageVersion
	if artifactID == groupID+".gradle.plugin" {
		implementation, err = markerImplementation(ctx, client, baseURL, groupID, artifactID, requested)
		if err != nil {
			return VersionInfo{}, err
		}
	}
	return VersionInfo{
		Implementation:       implementation,
		Ecosystem:            query.Ecosystem,
		Package:              query.Package,
		Version:              requested,
		Latest:               latest,
		Versions:             versions,
		PublishedAt:          publishedAtByVersion[requested],
		PublishedAtByVersion: publishedAtByVersion,
	}, nil
}

func fetchMavenMetadata(ctx context.Context, client HTTPClient, baseURL, groupID, artifactID, packageName string) (mavenMetadata, error) {
	metadataURL := baseURL + "/" + mavenArtifactPath(groupID, artifactID) + "/maven-metadata.xml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return mavenMetadata{}, err
	}
	req.Header.Set("Accept", "application/xml")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return mavenMetadata{}, fmt.Errorf("fetch Maven metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return mavenMetadata{}, PackageNotFoundError{Package: packageName}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mavenMetadata{}, fmt.Errorf("maven repository returned HTTP %d", resp.StatusCode)
	}

	var metadata mavenMetadata
	if err := mavenXMLDecoder(resp.Body).Decode(&metadata); err != nil {
		return mavenMetadata{}, fmt.Errorf("decode Maven metadata: %w", err)
	}
	return metadata, nil
}

func fetchMavenSearch(ctx context.Context, client HTTPClient, groupID, artifactID string) (mavenSearchResponse, error) {
	params := url.Values{}
	params.Set("q", fmt.Sprintf("g:%s AND a:%s", groupID, artifactID))
	params.Set("core", "gav")
	params.Set("rows", "200")
	params.Set("wt", "json")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://search.maven.org/solrsearch/select?"+params.Encode(), nil)
	if err != nil {
		return mavenSearchResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return mavenSearchResponse{}, fmt.Errorf("fetch Maven Search versions: %w", err)
	}
	var search mavenSearchResponse
	if err := decodeJSON(resp, &search); err != nil {
		return mavenSearchResponse{}, err
	}
	if len(search.Response.Docs) == 0 {
		return mavenSearchResponse{}, PackageNotFoundError{Package: groupID + ":" + artifactID}
	}
	return search, nil
}

func fetchMavenPublishedAt(ctx context.Context, client HTTPClient, baseURL, groupID, artifactID, version string) *time.Time {
	artifactPath := mavenArtifactPath(groupID, artifactID)
	pomName := url.PathEscape(artifactID) + "-" + url.PathEscape(version) + ".pom"
	pomURL := baseURL + "/" + artifactPath + "/" + url.PathEscape(version) + "/" + pomName
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, pomURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	publishedAt, err := http.ParseTime(resp.Header.Get("Last-Modified"))
	if err != nil {
		return nil
	}
	publishedAt = publishedAt.UTC()
	return &publishedAt
}

func mavenMetadataLatest(metadata mavenMetadata, versionSet map[string]struct{}) string {
	for _, candidate := range []string{metadata.Versioning.Release, metadata.Versioning.Latest} {
		candidate = strings.TrimSpace(candidate)
		if _, ok := versionSet[candidate]; ok {
			return candidate
		}
	}
	return ""
}

func mavenSearchPublishedAt(search mavenSearchResponse, versionSet map[string]struct{}) map[string]*time.Time {
	publishedAtByVersion := map[string]*time.Time{}
	for _, doc := range search.Response.Docs {
		if _, ok := versionSet[doc.Version]; ok {
			publishedAtByVersion[doc.Version] = mavenPublishedAt(doc.Timestamp)
		}
	}
	return publishedAtByVersion
}

func mavenArtifactPath(groupID, artifactID string) string {
	groupParts := strings.Split(groupID, ".")
	for i, part := range groupParts {
		groupParts[i] = url.PathEscape(part)
	}
	return strings.Join(groupParts, "/") + "/" + url.PathEscape(artifactID)
}

func splitMavenPackage(value string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("maven package must be groupId:artifactId, got %q", value)
	}
	return parts[0], parts[1], nil
}

func mavenPublishedAt(timestampMillis int64) *time.Time {
	if timestampMillis <= 0 {
		return nil
	}
	parsed := time.UnixMilli(timestampMillis).UTC()
	return &parsed
}

const PluginPortalURL = "https://plugins.gradle.org/m2"
const IntelliJReleasesURL = "https://www.jetbrains.com/intellij-repository/releases"

func markerImplementation(ctx context.Context, client HTTPClient, base, group, artifact, version string) (*models.PackageVersion, error) {
	path := base + "/" + mavenArtifactPath(group, artifact) + "/" + url.PathEscape(version) + "/" + url.PathEscape(artifact+"-"+version) + ".pom"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin marker POM returned HTTP %d", resp.StatusCode)
	}
	var pom struct {
		Dependencies []struct {
			Group    string `xml:"groupId"`
			Artifact string `xml:"artifactId"`
			Version  string `xml:"version"`
		} `xml:"dependencies>dependency"`
	}
	if err := mavenXMLDecoder(resp.Body).Decode(&pom); err != nil {
		return nil, err
	}
	if len(pom.Dependencies) != 1 {
		return nil, fmt.Errorf("expected one implementation dependency in plugin marker POM")
	}
	dep := pom.Dependencies[0]
	for _, field := range []string{dep.Group, dep.Artifact, dep.Version} {
		if field == "" || strings.ContainsAny(field, "$[](),:+/ ") || strings.HasSuffix(field, "-SNAPSHOT") || field == "LATEST" || field == "RELEASE" {
			return nil, fmt.Errorf("plugin marker requires literal implementation coordinates")
		}
	}
	return &models.PackageVersion{Ecosystem: models.EcosystemMaven, Package: dep.Group + ":" + dep.Artifact, Version: dep.Version}, nil
}

func mavenXMLDecoder(body io.Reader) *xml.Decoder {
	d := xml.NewDecoder(body)
	d.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		if strings.EqualFold(label, "us-ascii") {
			return input, nil
		}
		return nil, fmt.Errorf("unsupported Maven XML encoding %q", label)
	}
	return d
}
