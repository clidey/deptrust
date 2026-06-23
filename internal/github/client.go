package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/clidey/deptrust/internal/models"
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
		endpoint:   "https://api.github.com/advisories",
	}
}

func (c Client) Name() string {
	return "GitHub Advisory DB"
}

func (c Client) Supports(ecosystem models.Ecosystem) bool {
	return ecosystem.GitHubEcosystem() != ""
}

type advisory struct {
	GHSAID          string                  `json:"ghsa_id"`
	CVEID           string                  `json:"cve_id"`
	HTMLURL         string                  `json:"html_url"`
	Summary         string                  `json:"summary"`
	Description     string                  `json:"description"`
	Type            string                  `json:"type"`
	Severity        string                  `json:"severity"`
	Identifiers     []identifier            `json:"identifiers"`
	References      []string                `json:"references"`
	PublishedAt     *time.Time              `json:"published_at"`
	UpdatedAt       *time.Time              `json:"updated_at"`
	Vulnerabilities []advisoryVulnerability `json:"vulnerabilities"`
}

type identifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type advisoryVulnerability struct {
	Package struct {
		Ecosystem string `json:"ecosystem"`
		Name      string `json:"name"`
	} `json:"package"`
	VulnerableVersionRange string         `json:"vulnerable_version_range"`
	FirstPatchedVersion    patchedVersion `json:"first_patched_version"`
}

type patchedVersion struct {
	Identifier string `json:"identifier"`
}

func (p *patchedVersion) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil
	}

	var identifier string
	if err := json.Unmarshal(data, &identifier); err == nil {
		p.Identifier = identifier
		return nil
	}

	type patchedVersionObject patchedVersion
	var decoded patchedVersionObject
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	p.Identifier = decoded.Identifier
	return nil
}

func (c Client) Query(ctx context.Context, pkg models.PackageVersion) ([]models.Vulnerability, error) {
	if !c.Supports(pkg.Ecosystem) {
		return nil, nil
	}

	var out []models.Vulnerability
	for _, advisoryType := range []string{"reviewed", "malware"} {
		advisories, err := c.queryType(ctx, pkg, advisoryType)
		if err != nil {
			return nil, err
		}
		for _, item := range advisories {
			out = append(out, convertAdvisory(item, pkg))
		}
	}
	return out, nil
}

func (c Client) queryType(ctx context.Context, pkg models.PackageVersion, advisoryType string) ([]advisory, error) {
	params := url.Values{}
	params.Set("ecosystem", pkg.Ecosystem.GitHubEcosystem())
	params.Set("affects", pkg.Package+"@"+pkg.Version)
	params.Set("type", advisoryType)
	params.Set("per_page", "100")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "deptrust")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query GitHub advisories: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub advisories returned HTTP %d", resp.StatusCode)
	}

	var decoded []advisory
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode GitHub advisories response: %w", err)
	}
	return decoded, nil
}

func convertAdvisory(item advisory, pkg models.PackageVersion) models.Vulnerability {
	aliases, cves, ghsas := identifiers(item)
	refs := make([]models.Reference, 0, len(item.References)+1)
	if item.HTMLURL != "" {
		refs = append(refs, models.Reference{Type: "ADVISORY", URL: item.HTMLURL})
	}
	for _, ref := range item.References {
		if ref == "" {
			continue
		}
		refs = append(refs, models.Reference{Type: "REFERENCE", URL: ref})
	}

	ranges, fixed := affected(item.Vulnerabilities, pkg)
	return models.Vulnerability{
		ID:             item.GHSAID,
		Aliases:        aliases,
		CVEIDs:         cves,
		GHSAIDs:        ghsas,
		Severity:       strings.ToLower(item.Severity),
		Summary:        item.Summary,
		Details:        firstParagraph(item.Description),
		Source:         "GitHub Advisory DB",
		AdvisoryURL:    item.HTMLURL,
		AffectedRanges: ranges,
		FixedVersions:  fixed,
		References:     refs,
		PublishedAt:    item.PublishedAt,
		ModifiedAt:     item.UpdatedAt,
	}
}

func identifiers(item advisory) ([]string, []string, []string) {
	var aliases []string
	var cves []string
	var ghsas []string
	add := func(kind, value string) {
		if value == "" {
			return
		}
		aliases = appendUnique(aliases, value)
		switch strings.ToUpper(kind) {
		case "CVE":
			cves = appendUnique(cves, strings.ToUpper(value))
		case "GHSA":
			ghsas = appendUnique(ghsas, value)
		}
	}
	add("GHSA", item.GHSAID)
	add("CVE", item.CVEID)
	for _, identifier := range item.Identifiers {
		add(identifier.Type, identifier.Value)
	}
	return aliases, cves, ghsas
}

func affected(items []advisoryVulnerability, pkg models.PackageVersion) ([]string, []string) {
	var ranges []string
	var fixed []string
	for _, item := range items {
		if !strings.EqualFold(item.Package.Name, pkg.Package) {
			continue
		}
		if item.Package.Ecosystem != "" && item.Package.Ecosystem != pkg.Ecosystem.GitHubEcosystem() {
			continue
		}
		if item.VulnerableVersionRange != "" {
			ranges = append(ranges, item.VulnerableVersionRange)
		}
		if item.FirstPatchedVersion.Identifier != "" {
			fixed = appendUnique(fixed, item.FirstPatchedVersion.Identifier)
		}
	}
	return ranges, fixed
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}

func firstParagraph(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "\n\n")
	return strings.TrimSpace(parts[0])
}
