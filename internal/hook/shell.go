package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/clidey/deptrust/internal/app"
	"github.com/clidey/deptrust/internal/models"
	"github.com/clidey/deptrust/internal/risk"
)

type claudeHookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

type claudeHookOutput struct {
	HookSpecificOutput claudeHookDecision `json:"hookSpecificOutput"`
}

type claudeHookDecision struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

type packageRequest struct {
	Ecosystem string
	Name      string
	Version   string
}

func RunShell(ctx context.Context, service app.App, in io.Reader, out io.Writer) error {
	var input claudeHookInput
	if err := json.NewDecoder(in).Decode(&input); err != nil {
		return nil
	}
	if input.ToolName != "" && input.ToolName != "Bash" {
		return nil
	}

	requests := ParseInstallCommand(input.ToolInput.Command)
	for _, request := range requests {
		query, err := app.ParseQuery(request.Ecosystem, request.Name, request.Version)
		if err != nil {
			continue
		}
		result, err := service.CheckPackage(ctx, query)
		if err != nil {
			return deny(out, fmt.Sprintf("deptrust could not check %s %s: %v", request.Name, request.Version, err))
		}
		if result.Recommendation != risk.RecommendationAllow {
			reason := fmt.Sprintf("deptrust blocked %s %s@%s: recommendation=%s; %s", request.Ecosystem, request.Name, result.Version, result.Recommendation, result.Summary)
			return deny(out, reason)
		}
	}
	return nil
}

func ParseInstallCommand(command string) []packageRequest {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "npm":
		return parseNPM(fields[1:])
	case "pnpm":
		return parsePNPM(fields[1:])
	case "yarn":
		return parseYarn(fields[1:])
	case "bun":
		return parseBun(fields[1:])
	case "pip", "pip3":
		return parsePip(fields[1:])
	case "uv":
		return parseUV(fields[1:])
	case "cargo":
		return parseCargo(fields[1:])
	case "go":
		return parseGo(fields[1:])
	case "bundle":
		return parseBundle(fields[1:])
	case "dotnet":
		return parseDotnet(fields[1:])
	case "composer":
		return parseComposer(fields[1:])
	case "gem":
		return parseGem(fields[1:])
	case "mvn":
		return parseMaven(fields[1:])
	default:
		return nil
	}
}

func deny(out io.Writer, reason string) error {
	return json.NewEncoder(out).Encode(claudeHookOutput{
		HookSpecificOutput: claudeHookDecision{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "deny",
			PermissionDecisionReason: reason,
		},
	})
}

func parseNPM(args []string) []packageRequest {
	if len(args) == 0 || !oneOf(args[0], "install", "i", "add", "update") {
		return nil
	}
	return parseNameSpecs("npm", args[1:])
}

func parsePNPM(args []string) []packageRequest {
	if len(args) == 0 || !oneOf(args[0], "add", "install", "i", "update", "up") {
		return nil
	}
	return parseNameSpecs("npm", args[1:])
}

func parseYarn(args []string) []packageRequest {
	if len(args) == 0 || !oneOf(args[0], "add", "up", "upgrade") {
		return nil
	}
	return parseNameSpecs("npm", args[1:])
}

func parseBun(args []string) []packageRequest {
	if len(args) == 0 || !oneOf(args[0], "add", "install") {
		return nil
	}
	return parseNameSpecs("npm", args[1:])
}

func parsePip(args []string) []packageRequest {
	if len(args) == 0 || args[0] != "install" {
		return nil
	}
	var out []packageRequest
	for _, arg := range args[1:] {
		if skipPackageArg(arg) {
			continue
		}
		name, version := splitPythonSpec(arg)
		out = appendPackage(out, "pypi", name, version)
	}
	return out
}

func parseUV(args []string) []packageRequest {
	if len(args) == 0 {
		return nil
	}
	if args[0] == "add" {
		return parsePip(append([]string{"install"}, args[1:]...))
	}
	if len(args) > 1 && args[0] == "pip" && args[1] == "install" {
		return parsePip(args[1:])
	}
	return nil
}

func parseCargo(args []string) []packageRequest {
	if len(args) == 0 || args[0] != "add" {
		return nil
	}
	var out []packageRequest
	for _, arg := range args[1:] {
		if skipPackageArg(arg) {
			continue
		}
		name, version := splitAtVersion(arg)
		out = appendPackage(out, "cargo", name, version)
	}
	return out
}

func parseGo(args []string) []packageRequest {
	if len(args) == 0 || args[0] != "get" {
		return nil
	}
	var out []packageRequest
	for _, arg := range args[1:] {
		if skipPackageArg(arg) {
			continue
		}
		name, version := splitAtVersion(arg)
		out = appendPackage(out, "go", name, version)
	}
	return out
}

func parseBundle(args []string) []packageRequest {
	if len(args) < 2 || args[0] != "add" {
		return nil
	}
	version := ""
	for i, arg := range args {
		if oneOf(arg, "--version", "-v") && i+1 < len(args) {
			version = args[i+1]
		}
	}
	return []packageRequest{{Ecosystem: "rubygems", Name: args[1], Version: defaultVersion(version)}}
}

func parseDotnet(args []string) []packageRequest {
	if len(args) < 3 || args[0] != "add" || args[1] != "package" {
		return nil
	}
	version := ""
	for i, arg := range args {
		if arg == "--version" && i+1 < len(args) {
			version = args[i+1]
		}
	}
	return []packageRequest{{Ecosystem: "nuget", Name: args[2], Version: defaultVersion(version)}}
}

func parseComposer(args []string) []packageRequest {
	if len(args) == 0 || !oneOf(args[0], "require", "update") {
		return nil
	}
	var out []packageRequest
	for _, arg := range args[1:] {
		if skipPackageArg(arg) {
			continue
		}
		name, version, _ := strings.Cut(arg, ":")
		out = appendPackage(out, "packagist", name, version)
	}
	return out
}

func parseGem(args []string) []packageRequest {
	if len(args) < 2 || args[0] != "install" {
		return nil
	}
	version := ""
	for i, arg := range args {
		if oneOf(arg, "--version", "-v") && i+1 < len(args) {
			version = args[i+1]
		}
	}
	return []packageRequest{{Ecosystem: "rubygems", Name: args[1], Version: defaultVersion(version)}}
}

func parseMaven(args []string) []packageRequest {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-Dartifact=") {
			continue
		}
		artifact := strings.TrimPrefix(arg, "-Dartifact=")
		parts := strings.Split(artifact, ":")
		if len(parts) < 3 {
			continue
		}
		return []packageRequest{{Ecosystem: "maven", Name: parts[0] + ":" + parts[1], Version: defaultVersion(parts[2])}}
	}
	return nil
}

func parseNameSpecs(ecosystem string, args []string) []packageRequest {
	var out []packageRequest
	for _, arg := range args {
		if skipPackageArg(arg) {
			continue
		}
		name, version := splitAtVersion(arg)
		out = appendPackage(out, ecosystem, name, version)
	}
	return out
}

func appendPackage(out []packageRequest, ecosystem, name, version string) []packageRequest {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, "-") || strings.Contains(name, "/") && strings.HasPrefix(name, ".") {
		return out
	}
	return append(out, packageRequest{Ecosystem: ecosystem, Name: name, Version: defaultVersion(version)})
}

func splitAtVersion(spec string) (string, string) {
	if strings.HasPrefix(spec, "@") {
		index := strings.LastIndex(spec[1:], "@")
		if index >= 0 {
			index++
			return spec[:index], spec[index+1:]
		}
		return spec, models.LatestVersion
	}
	name, version, ok := strings.Cut(spec, "@")
	if !ok {
		return spec, models.LatestVersion
	}
	return name, version
}

func splitPythonSpec(spec string) (string, string) {
	name, version, ok := strings.Cut(spec, "==")
	if ok {
		return name, version
	}
	for _, sep := range []string{">=", "<=", "~=", ">", "<"} {
		if index := strings.Index(spec, sep); index > 0 {
			return spec[:index], models.LatestVersion
		}
	}
	return spec, models.LatestVersion
}

func skipPackageArg(arg string) bool {
	return arg == "" || strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, ".") || strings.HasPrefix(arg, "/")
}

func defaultVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return models.LatestVersion
	}
	return version
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}
