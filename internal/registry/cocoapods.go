package registry

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/clidey/deptrust/internal/models"
)

func resolveCocoaPods(ctx context.Context, client HTTPClient, query models.Query) (VersionInfo, error) {
	shard := cocoaPodsShard(query.Package)
	endpoint := fmt.Sprintf("https://cdn.cocoapods.org/all_pods_versions_%s_%s_%s.txt", shard[0:1], shard[1:2], shard[2:3])
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return VersionInfo{}, err
	}
	req.Header.Set("User-Agent", "deptrust")

	resp, err := client.Do(req)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("fetch CocoaPods CDN metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return VersionInfo{}, fmt.Errorf("CocoaPods CDN returned HTTP %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	var versions []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Split(line, "/")
		if len(fields) < 2 || !strings.EqualFold(fields[0], query.Package) {
			continue
		}
		versions = append(versions, fields[1:]...)
		break
	}
	if err := scanner.Err(); err != nil {
		return VersionInfo{}, fmt.Errorf("read CocoaPods CDN metadata: %w", err)
	}
	if len(versions) == 0 {
		return VersionInfo{}, PackageNotFoundError{Package: query.Package}
	}

	versionSet := map[string]struct{}{}
	for _, version := range versions {
		versionSet[version] = struct{}{}
	}
	sorted := sortedVersionKeys(versionSet)
	latest := sorted[0]
	requested := canonicalRequestedVersion(query.Version, latest, versionSet)
	if _, ok := versionSet[requested]; !ok {
		return VersionInfo{}, VersionNotFoundError{Package: query.Package, Version: strings.TrimSpace(query.Version), Latest: latest}
	}

	return VersionInfo{
		Ecosystem: query.Ecosystem,
		Package:   query.Package,
		Version:   requested,
		Latest:    latest,
		Versions:  sorted,
	}, nil
}

func cocoaPodsShard(name string) string {
	sum := md5.Sum([]byte(name))
	return hex.EncodeToString(sum[:])[:3]
}
