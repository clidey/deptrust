package osv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/clidey/deptrust/internal/models"
	"github.com/clidey/deptrust/internal/risk"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	httpClient HTTPClient
	endpoint   string
}

func New(client HTTPClient) Client {
	return Client{
		httpClient: client,
		endpoint:   "https://api.osv.dev/v1/query",
	}
}

type queryRequest struct {
	Version string       `json:"version"`
	Package queryPackage `json:"package"`
}

type queryPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type queryResponse struct {
	Vulns []vulnerability `json:"vulns"`
}

type vulnerability struct {
	ID               string      `json:"id"`
	Summary          string      `json:"summary"`
	Details          string      `json:"details"`
	Aliases          []string    `json:"aliases"`
	Modified         *time.Time  `json:"modified"`
	Published        *time.Time  `json:"published"`
	Severity         []severity  `json:"severity"`
	Affected         []affected  `json:"affected"`
	References       []reference `json:"references"`
	DatabaseSpecific struct {
		Severity string `json:"severity"`
	} `json:"database_specific"`
}

type severity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

type affected struct {
	Package struct {
		Name      string `json:"name"`
		Ecosystem string `json:"ecosystem"`
	} `json:"package"`
	Ranges           []affectedRange `json:"ranges"`
	Versions         []string        `json:"versions"`
	DatabaseSpecific struct {
		Source string `json:"source"`
	} `json:"database_specific"`
}

type affectedRange struct {
	Type   string       `json:"type"`
	Events []rangeEvent `json:"events"`
}

type rangeEvent struct {
	Introduced   string `json:"introduced,omitempty"`
	Fixed        string `json:"fixed,omitempty"`
	LastAffected string `json:"last_affected,omitempty"`
	Limit        string `json:"limit,omitempty"`
}

type reference struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func (c Client) Query(ctx context.Context, pkg models.PackageVersion) ([]models.Vulnerability, error) {
	payload, err := json.Marshal(queryRequest{
		Version: pkg.Version,
		Package: queryPackage{
			Name:      pkg.Package,
			Ecosystem: pkg.Ecosystem.OSVEcosystem(),
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query OSV: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OSV returned HTTP %d", resp.StatusCode)
	}

	var decoded queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode OSV response: %w", err)
	}

	out := make([]models.Vulnerability, 0, len(decoded.Vulns))
	for _, vuln := range decoded.Vulns {
		out = append(out, convertVulnerability(vuln, pkg))
	}
	return out, nil
}

func convertVulnerability(v vulnerability, pkg models.PackageVersion) models.Vulnerability {
	refs := make([]models.Reference, 0, len(v.References))
	for _, ref := range v.References {
		if ref.URL == "" {
			continue
		}
		refs = append(refs, models.Reference{Type: ref.Type, URL: ref.URL})
	}

	ranges, fixedVersions := affectedRanges(v.Affected, pkg)
	severity := bestSeverity(v)
	if severity == "unknown" {
		severity = inferSeverityFromAliases(v.Aliases)
	}
	cves, ghsas := advisoryIDs(v.ID, v.Aliases)

	return models.Vulnerability{
		ID:             v.ID,
		Aliases:        v.Aliases,
		CVEIDs:         cves,
		GHSAIDs:        ghsas,
		Severity:       severity,
		Summary:        v.Summary,
		Details:        firstParagraph(v.Details),
		Source:         "OSV",
		AdvisoryURL:    primaryAdvisoryURL(refs),
		AffectedRanges: ranges,
		FixedVersions:  fixedVersions,
		References:     refs,
		PublishedAt:    v.Published,
		ModifiedAt:     v.Modified,
	}
}

func advisoryIDs(id string, aliases []string) ([]string, []string) {
	var cves []string
	var ghsas []string
	addID := func(value string) {
		upper := strings.ToUpper(strings.TrimSpace(value))
		switch {
		case strings.HasPrefix(upper, "CVE-"):
			cves = appendUnique(cves, upper)
		case strings.HasPrefix(upper, "GHSA-"):
			ghsas = appendUnique(ghsas, value)
		}
	}
	addID(id)
	for _, alias := range aliases {
		addID(alias)
	}
	return cves, ghsas
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}

func primaryAdvisoryURL(refs []models.Reference) string {
	for _, ref := range refs {
		if strings.EqualFold(ref.Type, "ADVISORY") && ref.URL != "" {
			return ref.URL
		}
	}
	if len(refs) > 0 {
		return refs[0].URL
	}
	return ""
}

func bestSeverity(v vulnerability) string {
	best := "unknown"
	if v.DatabaseSpecific.Severity != "" {
		best = risk.NormalizeSeverity(v.DatabaseSpecific.Severity)
	}
	for _, sev := range v.Severity {
		normalized := normalizeOSVSeverity(sev)
		if risk.SeverityRank(normalized) > risk.SeverityRank(best) {
			best = normalized
		}
	}
	return best
}

func normalizeOSVSeverity(sev severity) string {
	score := strings.TrimSpace(sev.Score)
	if score == "" {
		return "unknown"
	}
	if strings.EqualFold(sev.Type, "CVSS_V3") || strings.HasPrefix(score, "CVSS:3.") {
		return cvssVectorSeverity(score)
	}
	if strings.EqualFold(sev.Type, "CVSS_V4") || strings.HasPrefix(score, "CVSS:4.") {
		return cvssVectorSeverity(score)
	}
	if numericSeverity(score) != "unknown" {
		return numericSeverity(score)
	}
	return risk.NormalizeSeverity(score)
}

func cvssVectorSeverity(vector string) string {
	upper := strings.ToUpper(vector)
	if strings.Contains(upper, "/S:C") && strings.Contains(upper, "/C:H") && strings.Contains(upper, "/I:H") {
		return "critical"
	}
	switch {
	case strings.Contains(upper, "/CR:H"):
		return "critical"
	case strings.Contains(upper, "/CR:M"), strings.Contains(upper, "/IR:H"), strings.Contains(upper, "/AR:H"):
		return "high"
	case strings.Contains(upper, "/AV:N") && strings.Contains(upper, "/PR:N") && strings.Contains(upper, "/UI:N"):
		return "high"
	default:
		return "unknown"
	}
}

func numericSeverity(value string) string {
	var score float64
	if _, err := fmt.Sscanf(value, "%f", &score); err != nil {
		return "unknown"
	}
	switch {
	case score >= 9:
		return "critical"
	case score >= 7:
		return "high"
	case score >= 4:
		return "medium"
	case score > 0:
		return "low"
	default:
		return "unknown"
	}
}

func inferSeverityFromAliases(aliases []string) string {
	for _, alias := range aliases {
		if strings.HasPrefix(strings.ToUpper(alias), "GHSA-") {
			return "unknown"
		}
	}
	return "unknown"
}

func affectedRanges(affected []affected, pkg models.PackageVersion) ([]string, []string) {
	var ranges []string
	var fixed []string
	seenFixed := map[string]struct{}{}

	for _, item := range affected {
		if item.Package.Name != "" && !strings.EqualFold(item.Package.Name, pkg.Package) {
			continue
		}
		if item.Package.Ecosystem != "" && item.Package.Ecosystem != pkg.Ecosystem.OSVEcosystem() {
			continue
		}
		for _, rng := range item.Ranges {
			var parts []string
			for _, event := range rng.Events {
				switch {
				case event.Introduced != "":
					parts = append(parts, "introduced "+event.Introduced)
				case event.Fixed != "":
					parts = append(parts, "fixed "+event.Fixed)
					if _, ok := seenFixed[event.Fixed]; !ok {
						fixed = append(fixed, event.Fixed)
						seenFixed[event.Fixed] = struct{}{}
					}
				case event.LastAffected != "":
					parts = append(parts, "last_affected "+event.LastAffected)
				case event.Limit != "":
					parts = append(parts, "limit "+event.Limit)
				}
			}
			if len(parts) > 0 {
				prefix := rng.Type
				if prefix == "" {
					prefix = "range"
				}
				ranges = append(ranges, prefix+": "+strings.Join(parts, ", "))
			}
		}
	}

	return ranges, fixed
}

func firstParagraph(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "\n\n")
	return strings.TrimSpace(parts[0])
}
