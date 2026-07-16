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
	Signals              []models.Signal       `json:"-"`
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
	case models.EcosystemGo:
		return resolveGo(ctx, r.client, query)
	case models.EcosystemRuby:
		return resolveRubyGems(ctx, r.client, query)
	case models.EcosystemNuGet:
		return resolveNuGet(ctx, r.client, query)
	case models.EcosystemMaven:
		return resolveMaven(ctx, r.client, query)
	case models.EcosystemPackagist:
		return resolvePackagist(ctx, r.client, query)
	case models.EcosystemPub:
		return resolvePub(ctx, r.client, query)
	case models.EcosystemCocoaPods:
		return resolveCocoaPods(ctx, r.client, query)
	case models.EcosystemHex:
		return resolveHex(ctx, r.client, query)
	case models.EcosystemHackage:
		return resolveHackage(ctx, r.client, query)
	case models.EcosystemGitHubActions:
		return resolveGitHubActions(ctx, r.client, query)
	default:
		return VersionInfo{}, fmt.Errorf("unsupported ecosystem %q", query.Ecosystem)
	}
}
