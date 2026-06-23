package registry

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestResolveRubyGemsLatest(t *testing.T) {
	client := fakeHTTPClient{responses: map[string]fakeResponse{
		"/api/v1/versions/rails.json": {
			status: http.StatusOK,
			body: `[
				{"number":"7.1.0","created_at":"2023-10-05T12:00:00.000Z"},
				{"number":"7.0.0","created_at":"2022-09-09T12:00:00.000Z"}
			]`,
		},
		"/api/v1/versions/rails/latest.json": {
			status: http.StatusOK,
			body:   `{"version":"7.1.0"}`,
		},
	}}

	got, err := resolveRubyGems(context.Background(), client, models.Query{
		Ecosystem: models.EcosystemRuby,
		Package:   "rails",
		Version:   models.LatestVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "7.1.0" || got.Latest != "7.1.0" {
		t.Fatalf("got version/latest %q/%q, want 7.1.0/7.1.0", got.Version, got.Latest)
	}
	if got.PublishedAt == nil {
		t.Fatal("PublishedAt = nil, want RubyGems created_at")
	}
}

func TestResolveNuGetExactVersionCaseInsensitive(t *testing.T) {
	client := fakeHTTPClient{responses: map[string]fakeResponse{
		"/v3-flatcontainer/newtonsoft.json/index.json": {
			status: http.StatusOK,
			body:   `{"versions":["12.0.3","13.0.3-beta","13.0.3"]}`,
		},
	}}

	got, err := resolveNuGet(context.Background(), client, models.Query{
		Ecosystem: models.EcosystemNuGet,
		Package:   "Newtonsoft.Json",
		Version:   "13.0.3-BETA",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "13.0.3-beta" {
		t.Fatalf("Version = %q, want canonical 13.0.3-beta", got.Version)
	}
	if got.Latest != "13.0.3" {
		t.Fatalf("Latest = %q, want 13.0.3", got.Latest)
	}
}

func TestResolveMavenLatest(t *testing.T) {
	query := mavenQueryKey("com.google.guava", "guava")
	client := fakeHTTPClient{responses: map[string]fakeResponse{
		"/solrsearch/select?" + query: {
			status: http.StatusOK,
			body: `{
				"response": {
					"docs": [
						{"g":"com.google.guava","a":"guava","v":"32.0.0","timestamp":1680000000000},
						{"g":"com.google.guava","a":"guava","v":"33.0.0","timestamp":1700000000000}
					]
				}
			}`,
		},
	}}

	got, err := resolveMaven(context.Background(), client, models.Query{
		Ecosystem: models.EcosystemMaven,
		Package:   "com.google.guava:guava",
		Version:   models.LatestVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "33.0.0" {
		t.Fatalf("Version = %q, want 33.0.0", got.Version)
	}
	if got.PublishedAt == nil {
		t.Fatal("PublishedAt = nil, want Maven timestamp")
	}
}

func TestResolveMavenRequiresGroupArtifact(t *testing.T) {
	_, err := resolveMaven(context.Background(), fakeHTTPClient{}, models.Query{
		Ecosystem: models.EcosystemMaven,
		Package:   "guava",
		Version:   models.LatestVersion,
	})
	if err == nil {
		t.Fatal("expected Maven coordinate validation error")
	}
}

func TestResolvePackagistLatest(t *testing.T) {
	client := fakeHTTPClient{responses: map[string]fakeResponse{
		"/p2/monolog/monolog.json": {
			status: http.StatusOK,
			body: `{"packages":{"monolog/monolog":[
				{"version":"3.10.0","time":"2026-01-02T08:56:05+00:00"},
				{"version":"2.9.0","time":"2025-03-24T10:00:00+00:00"}
			]}}`,
		},
	}}

	got, err := resolvePackagist(context.Background(), client, models.Query{
		Ecosystem: models.EcosystemPackagist,
		Package:   "monolog/monolog",
		Version:   models.LatestVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "3.10.0" || got.Latest != "3.10.0" {
		t.Fatalf("got version/latest %q/%q, want 3.10.0/3.10.0", got.Version, got.Latest)
	}
	if got.PublishedAt == nil {
		t.Fatal("PublishedAt = nil, want Packagist time")
	}
}

func TestResolvePubExactVersion(t *testing.T) {
	client := fakeHTTPClient{responses: map[string]fakeResponse{
		"/api/packages/http": {
			status: http.StatusOK,
			body: `{
				"latest":{"version":"1.6.0","published":"2025-11-10T18:27:56.434747Z"},
				"versions":[
					{"version":"1.5.0","published":"2025-08-01T00:00:00Z"},
					{"version":"1.6.0","published":"2025-11-10T18:27:56.434747Z"}
				]
			}`,
		},
	}}

	got, err := resolvePub(context.Background(), client, models.Query{
		Ecosystem: models.EcosystemPub,
		Package:   "http",
		Version:   "1.5.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.5.0" || got.Latest != "1.6.0" {
		t.Fatalf("got version/latest %q/%q, want 1.5.0/1.6.0", got.Version, got.Latest)
	}
	if got.PublishedAt == nil {
		t.Fatal("PublishedAt = nil, want pub.dev published")
	}
}

func TestResolveCocoaPodsLatest(t *testing.T) {
	path := "/all_pods_versions_" + strings.Join(strings.Split(cocoaPodsShard("AFNetworking"), ""), "_") + ".txt"
	client := fakeHTTPClient{responses: map[string]fakeResponse{
		path: {
			status: http.StatusOK,
			body:   "AFNetworking/4.0.0/4.0.1\nOtherPod/1.0.0\n",
		},
	}}

	got, err := resolveCocoaPods(context.Background(), client, models.Query{
		Ecosystem: models.EcosystemCocoaPods,
		Package:   "AFNetworking",
		Version:   models.LatestVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "4.0.1" || got.Latest != "4.0.1" {
		t.Fatalf("got version/latest %q/%q, want 4.0.1/4.0.1", got.Version, got.Latest)
	}
}

func TestResolveHexLatest(t *testing.T) {
	client := fakeHTTPClient{responses: map[string]fakeResponse{
		"/api/packages/plug": {
			status: http.StatusOK,
			body: `{
				"latest_version":"1.20.0",
				"latest_stable_version":"1.20.0",
				"releases":[
					{"version":"1.20.0","inserted_at":"2026-06-23T11:32:44.667819Z"},
					{"version":"1.19.0","inserted_at":"2026-01-01T00:00:00Z"}
				]
			}`,
		},
	}}

	got, err := resolveHex(context.Background(), client, models.Query{
		Ecosystem: models.EcosystemHex,
		Package:   "plug",
		Version:   models.LatestVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.20.0" || got.Latest != "1.20.0" {
		t.Fatalf("got version/latest %q/%q, want 1.20.0/1.20.0", got.Version, got.Latest)
	}
	if got.PublishedAt == nil {
		t.Fatal("PublishedAt = nil, want Hex inserted_at")
	}
}

func TestResolveHackageLatest(t *testing.T) {
	client := fakeHTTPClient{responses: map[string]fakeResponse{
		"/package/aeson/preferred.json": {
			status: http.StatusOK,
			body:   `{"normal-version":["2.2.3.0","2.3.0.0"],"deprecated-version":[]}`,
		},
	}}

	got, err := resolveHackage(context.Background(), client, models.Query{
		Ecosystem: models.EcosystemHackage,
		Package:   "aeson",
		Version:   models.LatestVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "2.3.0.0" || got.Latest != "2.3.0.0" {
		t.Fatalf("got version/latest %q/%q, want 2.3.0.0/2.3.0.0", got.Version, got.Latest)
	}
}

func TestResolveGitHubActionsExactTag(t *testing.T) {
	client := fakeHTTPClient{responses: map[string]fakeResponse{
		"/repos/actions/checkout/tags?per_page=100": {
			status: http.StatusOK,
			body:   `[{"name":"v7.0.0"},{"name":"v6.0.3"}]`,
		},
	}}

	got, err := resolveGitHubActions(context.Background(), client, models.Query{
		Ecosystem: models.EcosystemGitHubActions,
		Package:   "actions/checkout",
		Version:   "v6.0.3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "v6.0.3" || got.Latest != "v7.0.0" {
		t.Fatalf("got version/latest %q/%q, want v6.0.3/v7.0.0", got.Version, got.Latest)
	}
}

func TestResolveGitHubActionsAcceptsCommitSHA(t *testing.T) {
	client := fakeHTTPClient{responses: map[string]fakeResponse{
		"/repos/actions/checkout/tags?per_page=100": {
			status: http.StatusOK,
			body:   `[{"name":"v7.0.0"}]`,
		},
	}}

	sha := "9c091bbab93c267b02d269664d8ff18d57303105"
	got, err := resolveGitHubActions(context.Background(), client, models.Query{
		Ecosystem: models.EcosystemGitHubActions,
		Package:   "actions/checkout",
		Version:   sha,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != sha {
		t.Fatalf("Version = %q, want %q", got.Version, sha)
	}
}

func mavenQueryKey(groupID, artifactID string) string {
	params := url.Values{}
	params.Set("q", "g:"+groupID+" AND a:"+artifactID)
	params.Set("core", "gav")
	params.Set("rows", "200")
	params.Set("wt", "json")
	return params.Encode()
}
