package registry

import (
	"context"
	"net/http"
	"net/url"
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

func mavenQueryKey(groupID, artifactID string) string {
	params := url.Values{}
	params.Set("q", "g:"+groupID+" AND a:"+artifactID)
	params.Set("core", "gav")
	params.Set("rows", "200")
	params.Set("wt", "json")
	return params.Encode()
}
