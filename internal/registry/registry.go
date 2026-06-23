package registry

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/clidey/deptrust/internal/models"
)

type VersionInfo struct {
	Ecosystem            models.Ecosystem      `json:"ecosystem"`
	Package              string                `json:"package"`
	Version              string                `json:"version"`
	Latest               string                `json:"latest_version"`
	Versions             []string              `json:"versions,omitempty"`
	PublishedAt          *time.Time            `json:"published_at,omitempty"`
	PublishedAtByVersion map[string]*time.Time `json:"-"`
}

type Resolver interface {
	Resolve(ctx context.Context, query models.Query) (VersionInfo, error)
}

func New(client HTTPClient) Resolver {
	return resolver{client: client}
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type resolver struct {
	client HTTPClient
}

func (r resolver) Resolve(ctx context.Context, query models.Query) (VersionInfo, error) {
	switch query.Ecosystem {
	case models.EcosystemNPM:
		return resolveNPM(ctx, r.client, query)
	case models.EcosystemPyPI:
		return resolvePyPI(ctx, r.client, query)
	case models.EcosystemCargo:
		return resolveCargo(ctx, r.client, query)
	default:
		return VersionInfo{}, fmt.Errorf("unsupported ecosystem %q", query.Ecosystem)
	}
}
