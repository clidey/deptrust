package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/clidey/deptrust/internal/app"
	"github.com/clidey/deptrust/internal/buildinfo"
	"github.com/clidey/deptrust/internal/hook"
	"github.com/clidey/deptrust/internal/mcp"
	"github.com/clidey/deptrust/internal/risk"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitCode(err))
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stdout)
		return nil
	}

	service := app.New()
	switch args[0] {
	case "check":
		return runCheck(context.Background(), service, args[1:], stdout)
	case "suggest":
		return runSuggest(context.Background(), service, args[1:], stdout)
	case "compare":
		return runCompare(context.Background(), service, args[1:], stdout)
	case "mcp":
		return mcp.Serve(context.Background(), service, os.Stdin, stdout)
	case "setup":
		return runSetup(args[1:], os.Stdin, stdout, stderr)
	case "hook":
		return runHook(context.Background(), service, args[1:], os.Stdin, stdout)
	case "version":
		printVersion(stdout)
		return nil
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runSetup(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	var codex, claude bool
	for _, arg := range args {
		switch arg {
		case "--codex-mcp":
			codex = true
		case "--claude-code-mcp":
			claude = true
		default:
			return errors.New("usage: deptrust setup [--codex-mcp] [--claude-code-mcp]")
		}
	}
	guided := !codex && !claude

	executable := os.Args[0]
	if !filepath.IsAbs(executable) {
		var err error
		executable, err = exec.LookPath(executable)
		if err != nil {
			return fmt.Errorf("resolve deptrust executable: %w", err)
		}
	}
	executable, err := filepath.Abs(executable)
	if err != nil {
		return fmt.Errorf("resolve deptrust executable: %w", err)
	}

	reader := bufio.NewReader(stdin)
	fmt.Fprintf(stdout, "deptrust guided setup\n\nUsing binary: %s\n\n", executable)

	if commandAvailable("codex") {
		if guided {
			codex, err = promptYesNo(reader, stdout, "Register deptrust with Codex MCP?", true)
			if err != nil {
				return err
			}
		}
		if codex {
			if err := configureMCP("codex", []string{"mcp", "get", "deptrust"}, []string{"mcp", "remove", "deptrust"}, []string{"mcp", "add", "deptrust", "--", executable, "mcp"}, executable, stdout, stderr); err != nil {
				return err
			}
		}
	} else {
		fmt.Fprintln(stdout, "Codex CLI not found; skipped Codex MCP setup.")
	}

	if commandAvailable("claude") {
		if guided {
			claude, err = promptYesNo(reader, stdout, "Register deptrust with Claude Code MCP?", true)
			if err != nil {
				return err
			}
		}
		if claude {
			if err := configureMCP("claude", []string{"mcp", "get", "deptrust"}, []string{"mcp", "remove", "deptrust", "-s", "user"}, []string{"mcp", "add", "--transport", "stdio", "-s", "user", "deptrust", "--", executable, "mcp"}, executable, stdout, stderr); err != nil {
				return err
			}
		}
	} else {
		fmt.Fprintln(stdout, "Claude CLI not found; skipped Claude Code MCP setup.")
	}

	hooks := false
	if guided {
		hooks, err = promptYesNo(reader, stdout, "Install or update deptrust dependency safety hooks?", true)
		if err != nil {
			return err
		}
	}
	if err := configureHooks(executable, hooks, stdout); err != nil {
		return err
	}

	fmt.Fprintln(stdout, "Setup complete.")
	return nil
}

func promptYesNo(reader *bufio.Reader, stdout io.Writer, question string, defaultYes bool) (bool, error) {
	suffix := "[Y/n]"
	if !defaultYes {
		suffix = "[y/N]"
	}
	fmt.Fprintf(stdout, "%s %s ", question, suffix)
	answer, err := reader.ReadString('\n')
	if err != nil && len(answer) == 0 {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "" {
		return defaultYes, nil
	}
	return answer == "y" || answer == "yes", nil
}

func configureMCP(command string, getArgs, removeArgs, addArgs []string, executable string, stdout, stderr io.Writer) error {
	get := exec.Command(command, getArgs...)
	output, err := get.CombinedOutput()
	if err == nil && mcpMatchesExecutable(string(output), executable) {
		fmt.Fprintf(stdout, "deptrust is already connected to %s.\n", command)
		return nil
	}

	remove := exec.Command(command, removeArgs...)
	remove.Stdout = io.Discard
	remove.Stderr = io.Discard
	_ = remove.Run()

	add := exec.Command(command, addArgs...)
	add.Stdout = stdout
	add.Stderr = stderr
	if err := add.Run(); err != nil {
		return fmt.Errorf("configure %s MCP: %w", command, err)
	}
	fmt.Fprintf(stdout, "Connected deptrust to %s.\n", command)
	return nil
}

func mcpMatchesExecutable(output, executable string) bool {
	normalized := strings.ToLower(output)
	return strings.Contains(normalized, "command: "+strings.ToLower(executable)) && strings.Contains(normalized, "arg") && strings.Contains(normalized, "mcp")
}

func commandAvailable(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

func runHook(ctx context.Context, service app.App, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) != 1 || (args[0] != "shell" && args[0] != "tool") {
		return errors.New("usage: deptrust hook tool")
	}
	return hook.RunShell(ctx, service, stdin, stdout)
}

func runCheck(ctx context.Context, service app.App, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	remaining := flags.Args()
	if len(remaining) < 2 || len(remaining) > 3 {
		return errors.New("usage: deptrust check [--json] <ecosystem> <package> [version|latest]")
	}

	version := ""
	if len(remaining) == 3 {
		version = remaining[2]
	}
	query, err := app.ParseQuery(remaining[0], remaining[1], version)
	if err != nil {
		return err
	}

	result, err := service.CheckPackage(ctx, query)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(stdout, result)
	}
	printCheck(stdout, result.Summary, result.Recommendation, result.RiskScore)
	return nil
}

func runSuggest(ctx context.Context, service app.App, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("suggest", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	remaining := flags.Args()
	if len(remaining) != 2 {
		return errors.New("usage: deptrust suggest [--json] <ecosystem> <package>")
	}

	query, err := app.ParseQuery(remaining[0], remaining[1], "")
	if err != nil {
		return err
	}
	result, err := service.SuggestSafeVersion(ctx, query)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "%s\nrecommendation: %s\n", result.Summary, result.Recommendation)
	return nil
}

func runCompare(ctx context.Context, service app.App, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	remaining := flags.Args()
	if len(remaining) != 4 {
		return errors.New("usage: deptrust compare [--json] <ecosystem> <package> <from-version> <to-version>")
	}

	query, err := app.ParseQuery(remaining[0], remaining[1], "")
	if err != nil {
		return err
	}
	result, err := service.CompareVersions(ctx, query, remaining[2], remaining[3])
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "%s\nrecommendation: %s\nnext_action: %s\n", result.Summary, result.Recommendation, result.NextAction)
	return nil
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printCheck(w io.Writer, summary, recommendation string, score int) {
	fmt.Fprintf(w, "%s\nrecommendation: %s\nrisk_score: %d\n", summary, recommendation, score)
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "deptrust %s\ncommit: %s\nbuilt: %s\n", buildinfo.EffectiveVersion(), buildinfo.Commit, buildinfo.Date)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, strings.TrimSpace(`
deptrust checks package versions for known vulnerabilities.

Usage:
  deptrust check [--json] <ecosystem> <package> [version|latest]
  deptrust suggest [--json] <ecosystem> <package>
  deptrust compare [--json] <ecosystem> <package> <from-version> <to-version>
  deptrust mcp
  deptrust setup [--codex-mcp] [--claude-code-mcp]
  deptrust hook tool
  deptrust version

Examples:
  deptrust check npm lodash 4.17.20
  deptrust check --json pypi requests latest
  deptrust suggest cargo serde
  deptrust check go golang.org/x/crypto latest
  deptrust check rubygems rails latest
  deptrust check nuget Newtonsoft.Json latest
  deptrust check maven org.apache.logging.log4j:log4j-core latest
  deptrust check packagist monolog/monolog latest
  deptrust check pub http latest
  deptrust check cocoapods AFNetworking latest
  deptrust check hex plug latest
  deptrust check hackage aeson latest
  deptrust check github-actions actions/checkout v7.0.0
  deptrust compare npm lodash 4.17.20 4.17.21

Supported ecosystems: npm, pypi, cargo, go, rubygems, nuget, maven, packagist, pub, cocoapods, hex, hackage, github-actions

GitHub auth: set DEPTRUST_GITHUB_TOKEN in CI, or use DEPTRUST_GITHUB_AUTH=gh locally. Tokens are never stored.
`))
}

func exitCode(err error) int {
	message := err.Error()
	if strings.Contains(message, "provider") {
		return 2
	}
	if strings.Contains(message, "not found") {
		return 3
	}
	if strings.Contains(message, risk.RecommendationBlock) {
		return 10
	}
	return 1
}
