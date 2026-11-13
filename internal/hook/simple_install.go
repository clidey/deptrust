package hook

import (
	"errors"
	"regexp"
	"strings"
)

const simpleInstallReason = "deptrust could not fully inspect this install command. Retry with one standalone install command containing only literal package names and versions (for example: pnpm add lodash@4.17.21). Run directory changes separately; do not use redirects, command chaining, quotes, variables, substitutions, or unsupported options. Preserve required install options/source context by running an explicit source-aware check instead of silently dropping them"

// This is a deliberately restricted input format, not a shell parser. The
// detection patterns only identify likely installs needing simplification; they
// do not establish coverage for arbitrary shell programs or disguised commands.
var installManagerRE = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_])(?:npm|pnpm|yarn|bun|pip3?|uv|cargo|go|bundle|dotnet|composer|gem|mvn)(?:\.exe|\.cmd)?(?:$|[^a-z0-9_.-])`)
var installVerbRE = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_])(?:install|i|add|update|up|upgrade|get|require|dependency:get)(?:$|[^a-z0-9_])`)
var literalInstallRE = regexp.MustCompile(`^[A-Za-z0-9_./@:=+ \t-]+$`)
var packageNameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
var moduleNameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./-]*$`)
var literalVersionRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.+-]*$`)

func ParseInstallCommand(command string) ([]packageRequest, error) {
	if !installManagerRE.MatchString(command) || !installVerbRE.MatchString(command) {
		return nil, nil
	}
	retry := func() ([]packageRequest, error) { return nil, errors.New(simpleInstallReason) }
	if len(command) > 64*1024 || !literalInstallRE.MatchString(command) {
		return retry()
	}
	fields := strings.Fields(command)
	if len(fields) < 2 {
		return retry()
	}
	manager, verb := fields[0], fields[1]
	args := fields[2:]
	// These commands install from a manifest/lockfile, with no explicit package
	// identity to extract. Preserve the hook's existing scope for those forms.
	if len(args) == 0 && verb == "install" && oneOf(manager, "yarn", "bundle", "composer") {
		return nil, nil
	}
	switch manager {
	case "npm":
		if !oneOf(verb, "install", "i", "add", "update") {
			return retry()
		}
	case "pnpm":
		if !oneOf(verb, "install", "i", "add", "update", "up") {
			return retry()
		}
	case "yarn":
		if !oneOf(verb, "add", "up", "upgrade") {
			return retry()
		}
	case "bun":
		if !oneOf(verb, "add", "install") {
			return retry()
		}
	case "pip", "pip3":
		if verb != "install" {
			return retry()
		}
	case "uv":
		if verb == "pip" && len(args) > 0 && args[0] == "install" {
			args = args[1:]
		} else if verb != "add" {
			return retry()
		}
	case "cargo", "bundle":
		if verb != "add" {
			return retry()
		}
	case "go":
		if verb != "get" {
			return retry()
		}
	case "dotnet":
		if verb != "add" || len(args) == 0 || args[0] != "package" {
			return retry()
		}
		args = args[1:]
	case "composer":
		if !oneOf(verb, "require", "update") {
			return retry()
		}
	case "gem":
		if verb != "install" {
			return retry()
		}
	case "mvn":
		if verb != "dependency:get" || len(args) != 1 || !strings.HasPrefix(args[0], "-Dartifact=") || len(strings.Split(strings.TrimPrefix(args[0], "-Dartifact="), ":")) != 3 || strings.HasSuffix(args[0], ":") {
			return retry()
		}
		args = nil // The artifact is validated as a parsed request below.
	default:
		return retry()
	}
	if oneOf(manager, "gem", "bundle", "dotnet") {
		if len(args) != 1 && len(args) != 3 {
			return retry()
		}
		if len(args) == 3 && args[1] != "--version" && (manager == "dotnet" || args[1] != "-v") {
			return retry()
		}
		if len(args) == 3 && !literalVersionRE.MatchString(args[2]) {
			return retry()
		}
		args = args[:1]
	}
	for _, arg := range args {
		// Never silently discard an option or a local path as a package.
		if strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, ".") || strings.HasPrefix(arg, "/") || strings.HasSuffix(arg, "@") || strings.HasSuffix(arg, ":") || strings.HasSuffix(arg, "=") {
			return retry()
		}
	}
	requests := parseLiteralInstallCommand(command)
	if manager != "mvn" && len(requests) != len(args) {
		return retry()
	}
	if manager == "mvn" && len(requests) != 1 {
		return retry()
	}
	for _, request := range requests {
		if !simplePackageName(request.Ecosystem, request.Name) || !literalVersionRE.MatchString(request.Version) {
			return retry()
		}
	}
	return requests, nil
}

func simplePackageName(ecosystem, name string) bool {
	switch ecosystem {
	case "npm":
		if strings.HasPrefix(name, "@") {
			scope, pkg, ok := strings.Cut(name[1:], "/")
			return ok && packageNameRE.MatchString(scope) && packageNameRE.MatchString(pkg)
		}
		return packageNameRE.MatchString(name)
	case "go":
		return moduleNameRE.MatchString(name) && !strings.Contains(name, "..") && !strings.HasSuffix(name, "/")
	case "maven":
		group, artifact, ok := strings.Cut(name, ":")
		return ok && packageNameRE.MatchString(group) && packageNameRE.MatchString(artifact)
	case "packagist":
		vendor, pkg, ok := strings.Cut(name, "/")
		return ok && packageNameRE.MatchString(vendor) && packageNameRE.MatchString(pkg)
	default:
		return packageNameRE.MatchString(name)
	}
}
