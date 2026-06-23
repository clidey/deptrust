package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/clidey/deptrust/internal/app"
	"github.com/clidey/deptrust/internal/buildinfo"
)

const protocolVersion = "2025-11-25"
const ecosystemDescription = "Package ecosystem: npm, pypi, cargo, go, rubygems, nuget, maven, packagist, pub, cocoapods, hex, hackage, or github-actions."

const serverInstructions = "Use this server to vet dependencies for known vulnerabilities BEFORE acting on them. " +
	"Trigger it whenever you are about to add a new dependency, bump or pin a version, recommend an upgrade, " +
	"resolve a Dependabot/renovate PR, or audit a lockfile or manifest (package.json, requirements.txt, go.mod, " +
	"Cargo.toml, Gemfile, pom.xml, etc.) -- and whenever the user asks whether a package or version is 'safe', " +
	"'okay to update', or 'trustworthy'. Do this even for well-known, popular packages; popularity is not safety. " +
	"Prefer these tools over `npm audit`, `pip-audit`, web search, or training knowledge, which may be stale or incomplete.\n\n" +
	"Workflow:\n" +
	"- check_package: vet one ecosystem+package+version before installing or recommending it.\n" +
	"- suggest_safe_version: when a version is blocked or unknown, find the newest version with an allow recommendation.\n" +
	"- compare_versions: when moving between two known versions, confirm the target actually improves risk.\n\n" +
	"Recommendations: 'block' = do not install that version; 'review' = ask the user or pick a safer version; " +
	"'allow' = no blocking known vulnerability found; 'unknown' = provider failure or incomplete assessment, do NOT treat as safe. " +
	"When reporting back, include the package, version, recommendation, and the highest-severity advisory IDs found. " +
	"Boundaries: this checks known public advisories and registry metadata; it does not prove a package is safe, " +
	"download tarballs, or detect all malware."

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type checkArgs struct {
	Ecosystem string `json:"ecosystem"`
	Package   string `json:"package"`
	Version   string `json:"version,omitempty"`
}

type suggestArgs struct {
	Ecosystem string `json:"ecosystem"`
	Package   string `json:"package"`
}

type compareArgs struct {
	Ecosystem   string `json:"ecosystem"`
	Package     string `json:"package"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
}

func Serve(ctx context.Context, service app.App, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			if err := encoder.Encode(errorResponse(nil, -32700, "parse error")); err != nil {
				return err
			}
			continue
		}

		if len(req.ID) == 0 {
			continue
		}

		resp := handle(ctx, service, req)
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}

	return scanner.Err()
}

func handle(ctx context.Context, service app.App, req request) response {
	switch req.Method {
	case "initialize":
		return response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": protocolVersion,
				"serverInfo": map[string]any{
					"name":    "deptrust",
					"version": buildinfo.EffectiveVersion(),
				},
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"instructions": serverInstructions,
			},
		}
	case "tools/list":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": tools()}}
	case "tools/call":
		return callTool(ctx, service, req)
	default:
		return errorResponse(req.ID, -32601, fmt.Sprintf("method %q not found", req.Method))
	}
}

func callTool(ctx context.Context, service app.App, req request) response {
	var params toolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "invalid tool call params")
	}

	switch params.Name {
	case "check_package":
		var args checkArgs
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return toolError(req.ID, "invalid check_package arguments")
		}
		query, err := app.ParseQuery(args.Ecosystem, args.Package, args.Version)
		if err != nil {
			return toolError(req.ID, err.Error())
		}
		result, err := service.CheckPackage(ctx, query)
		if err != nil {
			return toolError(req.ID, err.Error())
		}
		compact := compactCheck(result)
		return toolResult(req.ID, compactToolText(result.Summary, compact.FullResponseCommand), compact)
	case "suggest_safe_version":
		var args suggestArgs
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return toolError(req.ID, "invalid suggest_safe_version arguments")
		}
		query, err := app.ParseQuery(args.Ecosystem, args.Package, "")
		if err != nil {
			return toolError(req.ID, err.Error())
		}
		result, err := service.SuggestSafeVersion(ctx, query)
		if err != nil {
			return toolError(req.ID, err.Error())
		}
		compact := compactSuggest(result)
		return toolResult(req.ID, compactToolText(result.Summary, compact.FullResponseCommand), compact)
	case "compare_versions":
		var args compareArgs
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return toolError(req.ID, "invalid compare_versions arguments")
		}
		query, err := app.ParseQuery(args.Ecosystem, args.Package, "")
		if err != nil {
			return toolError(req.ID, err.Error())
		}
		result, err := service.CompareVersions(ctx, query, args.FromVersion, args.ToVersion)
		if err != nil {
			return toolError(req.ID, err.Error())
		}
		compact := compactCompare(result)
		return toolResult(req.ID, compactToolText(result.Summary, compact.FullResponseCommand), compact)
	default:
		return toolError(req.ID, fmt.Sprintf("unknown tool %q", params.Name))
	}
}

func toolResult(id json.RawMessage, summary string, structured any) response {
	return response{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": summary},
			},
			"structuredContent": structured,
		},
	}
}

func compactToolText(summary, fullResponseCommand string) string {
	if fullResponseCommand == "" {
		return summary
	}
	return summary + "\n\nMCP returns a compact safety result by default. If the user asks to see full advisory details, run: " + fullResponseCommand
}

func toolError(id json.RawMessage, message string) response {
	return response{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"isError": true,
			"content": []map[string]any{
				{"type": "text", "text": message},
			},
		},
	}
}

func errorResponse(id json.RawMessage, code int, message string) response {
	return response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

func tools() []map[string]any {
	return []map[string]any{
		{
			"name":        "check_package",
			"description": "Vet a dependency before adding, installing, pinning, or recommending it. Checks whether a package version has known vulnerabilities across OSV and GitHub advisories and returns an install recommendation (allow, review, block, unknown). Use this for every new dependency or version bump, including popular packages.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ecosystem": map[string]any{
						"type":        "string",
						"description": ecosystemDescription,
					},
					"package": map[string]any{
						"type":        "string",
						"description": "Package name.",
					},
					"version": map[string]any{
						"type":        "string",
						"description": "Exact version or latest. Defaults to latest.",
					},
				},
				"required": []string{"ecosystem", "package"},
			},
			"outputSchema": checkOutputSchema(),
		},
		{
			"name":        "suggest_safe_version",
			"description": "Find a safe version to install when a requested version is blocked, unknown, or you need the newest non-vulnerable release. Checks package versions across OSV and GitHub advisories and suggests the newest version with an allow recommendation.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ecosystem": map[string]any{
						"type":        "string",
						"description": ecosystemDescription,
					},
					"package": map[string]any{
						"type":        "string",
						"description": "Package name.",
					},
				},
				"required": []string{"ecosystem", "package"},
			},
			"outputSchema": map[string]any{"type": "object"},
		},
		{
			"name":        "compare_versions",
			"description": "Decide whether an upgrade is worth it when moving between two known versions (e.g. evaluating a Dependabot/renovate bump). Compares two package versions and shows whether the target version improves risk.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ecosystem": map[string]any{
						"type":        "string",
						"description": ecosystemDescription,
					},
					"package": map[string]any{
						"type":        "string",
						"description": "Package name.",
					},
					"from_version": map[string]any{
						"type":        "string",
						"description": "Current exact version.",
					},
					"to_version": map[string]any{
						"type":        "string",
						"description": "Target exact version.",
					},
				},
				"required": []string{"ecosystem", "package", "from_version", "to_version"},
			},
			"outputSchema": map[string]any{"type": "object"},
		},
	}
}

func checkOutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ecosystem":                     map[string]any{"type": "string"},
			"package":                       map[string]any{"type": "string"},
			"version":                       map[string]any{"type": "string"},
			"latest_version":                map[string]any{"type": "string"},
			"known_vulnerabilities_found":   map[string]any{"type": "boolean"},
			"safe_to_use":                   map[string]any{"type": "boolean"},
			"should_install":                map[string]any{"type": "boolean"},
			"risk_score":                    map[string]any{"type": "integer"},
			"classification":                map[string]any{"type": "string"},
			"recommendation":                map[string]any{"type": "string"},
			"reason":                        map[string]any{"type": "string"},
			"next_action":                   map[string]any{"type": "string"},
			"summary":                       map[string]any{"type": "string"},
			"vulnerability_count":           map[string]any{"type": "integer"},
			"vulnerability_counts":          map[string]any{"type": "object"},
			"highest_severity":              map[string]any{"type": "string"},
			"signals":                       map[string]any{"type": "array"},
			"provider_errors":               map[string]any{"type": "array"},
			"checked_providers":             map[string]any{"type": "array"},
			"skipped_providers":             map[string]any{"type": "array"},
			"advisory_coverage":             map[string]any{"type": "string"},
			"advisory_coverage_reason":      map[string]any{"type": "string"},
			"resolved_from_version_request": map[string]any{"type": "string"},
			"full_response_command":         map[string]any{"type": "string"},
		},
	}
}
