package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReconcileHookReplacesLegacyDeptrustHook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := map[string]any{
		"model": "sonnet",
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash|Edit|Write|MultiEdit",
					"hooks": []any{
						map[string]any{"type": "command", "command": "/Users/example/.local/bin/deptrust"},
					},
				},
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{"type": "command", "command": "codegraph prompt-hook"},
					},
				},
			},
		},
	}
	writeConfig(t, path, original)

	target := hookTarget{
		path: path, matcher: claudeHookMatcher, command: "/opt/homebrew/bin/deptrust", args: []string{"hook", "shell"},
	}
	if _, err := reconcileHook(target); err != nil {
		t.Fatal(err)
	}

	config := readConfig(t, path)
	if got := config["model"]; got != "sonnet" {
		t.Fatalf("model = %v, want sonnet", got)
	}
	preToolUse := config["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(preToolUse) != 2 {
		t.Fatalf("PreToolUse entries = %d, want 2", len(preToolUse))
	}
	if got := preToolUse[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"]; got != "codegraph prompt-hook" {
		t.Fatalf("unrelated hook command = %v", got)
	}
	entry := preToolUse[1].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if got := entry["command"]; got != "/opt/homebrew/bin/deptrust" {
		t.Fatalf("deptrust hook command = %v", got)
	}
	args := entry["args"].([]any)
	if len(args) != 2 || args[0] != "hook" || args[1] != "shell" {
		t.Fatalf("deptrust hook args = %#v", args)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := reconcileHook(target)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("reconciling an identical hook reported a change")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("reconciling an identical hook rewrote the config")
	}
}

func TestMCPMatchesExecutable(t *testing.T) {
	executable := "/opt/homebrew/bin/deptrust"
	for _, output := range []string{
		"command: /opt/homebrew/bin/deptrust\nargs: mcp\n",
		"Command: /opt/homebrew/bin/deptrust\nArgs: mcp\n",
	} {
		if !mcpMatchesExecutable(output, executable) {
			t.Fatalf("expected MCP output to match: %q", output)
		}
	}
	if mcpMatchesExecutable("command: /Users/example/.local/bin/deptrust\nargs: mcp\n", executable) {
		t.Fatal("stale MCP path matched")
	}
}

func TestReconcileHookCreatesMissingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")
	if _, err := reconcileHook(hookTarget{path: path, matcher: claudeHookMatcher, command: "/tmp/deptrust", args: []string{"hook", "shell"}}); err != nil {
		t.Fatal(err)
	}
	config := readConfig(t, path)
	preToolUse := config["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(preToolUse) != 1 {
		t.Fatalf("PreToolUse entries = %d, want 1", len(preToolUse))
	}
}

func writeConfig(t *testing.T, path string, config map[string]any) {
	t.Helper()
	contents, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
}

func readConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]any{}
	if err := json.Unmarshal(contents, &config); err != nil {
		t.Fatal(err)
	}
	return config
}
