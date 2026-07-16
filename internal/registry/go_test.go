package registry

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

type fakeHTTPClient struct {
	responses map[string]fakeResponse
}

type fakeResponse struct {
	status int
	body   string
	header http.Header
}

func (f fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	key := req.URL.Path
	if req.URL.RawQuery != "" {
		key += "?" + req.URL.RawQuery
	}
	response, ok := f.responses[key]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	}
	return &http.Response{
		StatusCode: response.status,
		Body:       io.NopCloser(strings.NewReader(response.body)),
		Header:     response.header,
	}, nil
}

func TestResolveGoLatest(t *testing.T) {
	client := fakeHTTPClient{responses: map[string]fakeResponse{
		"/github.com/gin-gonic/gin/@v/list": {
			status: http.StatusOK,
			body:   "v1.9.0\nv1.10.0\n",
		},
		"/github.com/gin-gonic/gin/@v/v1.10.0.info": {
			status: http.StatusOK,
			body:   `{"Version":"v1.10.0","Time":"2024-05-07T13:00:00Z"}`,
		},
	}}

	got, err := resolveGo(context.Background(), client, models.Query{
		Ecosystem: models.EcosystemGo,
		Package:   "github.com/gin-gonic/gin",
		Version:   models.LatestVersion,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got.Version != "v1.10.0" {
		t.Fatalf("Version = %q, want v1.10.0", got.Version)
	}
	if got.Latest != "v1.10.0" {
		t.Fatalf("Latest = %q, want v1.10.0", got.Latest)
	}
	if got.PublishedAt == nil {
		t.Fatal("PublishedAt = nil, want latest timestamp")
	}
}

func TestResolveGoPseudoVersionDirectly(t *testing.T) {
	pseudoVersion := "v0.48.1-0.20260715233119-591dfa620de7"
	client := fakeHTTPClient{responses: map[string]fakeResponse{
		"/golang.org/x/tools/@v/" + pseudoVersion + ".info": {
			status: http.StatusOK,
			body:   `{"Version":"v0.48.1-0.20260715233119-591dfa620de7","Time":"2026-07-15T23:31:19Z"}`,
		},
	}}

	got, err := resolveGo(context.Background(), client, models.Query{
		Ecosystem: models.EcosystemGo,
		Package:   "golang.org/x/tools",
		Version:   pseudoVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != pseudoVersion {
		t.Fatalf("Version = %q, want %q", got.Version, pseudoVersion)
	}
}

func TestResolveGoExactVersion(t *testing.T) {
	client := fakeHTTPClient{responses: map[string]fakeResponse{
		"/github.com/gin-gonic/gin/@v/list": {
			status: http.StatusOK,
			body:   "v1.9.0\nv1.10.0\n",
		},
		"/github.com/gin-gonic/gin/@latest": {
			status: http.StatusOK,
			body:   `{"Version":"v1.10.0","Time":"2024-05-07T13:00:00Z"}`,
		},
		"/github.com/gin-gonic/gin/@v/v1.9.0.info": {
			status: http.StatusOK,
			body:   `{"Version":"v1.9.0","Time":"2023-08-01T13:00:00Z"}`,
		},
	}}

	got, err := resolveGo(context.Background(), client, models.Query{
		Ecosystem: models.EcosystemGo,
		Package:   "github.com/gin-gonic/gin",
		Version:   "v1.9.0",
	})
	if err != nil {
		t.Fatal(err)
	}

	if got.Version != "v1.9.0" {
		t.Fatalf("Version = %q, want v1.9.0", got.Version)
	}
	if got.PublishedAt == nil {
		t.Fatal("PublishedAt = nil, want exact version timestamp")
	}
}

func TestEscapeGoModulePath(t *testing.T) {
	got := escapeGoModulePath("github.com/Azure/azure-sdk-for-go")
	want := "github.com/!azure/azure-sdk-for-go"
	if got != want {
		t.Fatalf("escapeGoModulePath() = %q, want %q", got, want)
	}
}
