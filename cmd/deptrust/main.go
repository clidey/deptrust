package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/clidey/deptrust/internal/app"
	"github.com/clidey/deptrust/internal/buildinfo"
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
	case "mcp":
		return mcp.Serve(context.Background(), service, os.Stdin, stdout)
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

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printCheck(w io.Writer, summary, recommendation string, score int) {
	fmt.Fprintf(w, "%s\nrecommendation: %s\nrisk_score: %d\n", summary, recommendation, score)
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "deptrust %s\ncommit: %s\nbuilt: %s\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, strings.TrimSpace(`
deptrust checks package versions for known vulnerabilities.

Usage:
  deptrust check [--json] <ecosystem> <package> [version|latest]
  deptrust suggest [--json] <ecosystem> <package>
  deptrust mcp
  deptrust version

Examples:
  deptrust check npm lodash 4.17.20
  deptrust check --json pypi requests latest
  deptrust suggest cargo serde

Supported ecosystems: npm, pypi, cargo
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
