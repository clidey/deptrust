package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRefreshIntegrationsPreservesSettingsAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	write := func(name, value string) string {
		path := filepath.Join(home, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	codex := write(".codex/config.toml", "# keep this comment\nmodel = 'example'\n[mcp_servers.deptrust]\ncommand = '/old/bin/deptrust' # path comment\nargs = ['mcp']\nstartup_timeout_sec = 40\n[mcp_servers.deptrust.env]\nEXAMPLE = 'keep'\n[mcp_servers.other]\ncommand = 'other-tool'\n")
	claude := write(".claude.json", `{"theme":"dark","mcpServers":{"deptrust":{"command":"/old/bin/deptrust","args":["mcp"],"env":{"EXAMPLE":"keep"},"timeout":40},"other":{"command":"other-tool"}},"projects":{"/example":{"mcpServers":{"deptrust":{"command":"project-specific"}}}}}`)
	hooks := write(".codex/hooks.json", `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"DEPTRUST_GITHUB_AUTH=gh '/old/bin/deptrust' hook shell","timeout":45},{"type":"command","command":"other-hook"}]}]}}`)
	newPath := filepath.Join(home, "new bin", "deptrust")
	var out bytes.Buffer
	if err := refreshIntegrations(newPath, &out); err != nil {
		t.Fatal(err)
	}
	read := func(path string) []byte {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	got := string(read(codex))
	if !strings.Contains(got, "# keep this comment") || !strings.Contains(got, "# path comment") || !strings.Contains(got, "startup_timeout_sec = 40") || !strings.Contains(got, "EXAMPLE = 'keep'") || strings.Contains(got, "/old/bin/deptrust") {
		t.Fatal(got)
	}
	config := readConfig(t, claude)
	entry := config["mcpServers"].(map[string]any)["deptrust"].(map[string]any)
	if entry["command"] != newPath || entry["env"].(map[string]any)["EXAMPLE"] != "keep" || entry["timeout"] != float64(40) || config["theme"] != "dark" {
		t.Fatal("settings changed")
	}
	if !strings.Contains(string(read(claude)), "project-specific") {
		t.Fatal("project configuration changed")
	}
	if !strings.Contains(string(read(hooks)), "DEPTRUST_GITHUB_AUTH=gh ") || !strings.Contains(string(read(hooks)), "other-hook") || !strings.Contains(string(read(hooks)), `"timeout": 45`) {
		t.Fatal(string(read(hooks)))
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatal("enabled an absent Claude hook")
	}
	before := [][]byte{read(codex), read(claude), read(hooks)}
	if err := refreshIntegrations(newPath, &out); err != nil {
		t.Fatal(err)
	}
	for i, path := range []string{codex, claude, hooks} {
		if !bytes.Equal(before[i], read(path)) {
			t.Fatalf("second refresh rewrote %s", path)
		}
	}
}

func TestRefreshCodexTOMLLayouts(t *testing.T) {
	for _, source := range []string{
		"[mcp_servers.deptrust]\ncommand = '/old/deptrust'\nargs = ['mcp']\n",
		"mcp_servers.deptrust.command = '/old/deptrust'\nmcp_servers.deptrust.args = ['mcp']\n",
		"mcp_servers = { deptrust = { command = '/old/deptrust', args = ['mcp'] } }\n",
	} {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		if err := refreshCodexMCP(path, "/new/deptrust"); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "/new/deptrust") || strings.Contains(string(b), "/old/deptrust") {
			t.Fatal(string(b))
		}
	}
}

func TestRefreshDoesNotEnableIntegrationsOrChangeCustomServers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := refreshIntegrations("/new/deptrust", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("created configuration: %v, %v", entries, err)
	}
	for _, server := range []string{`{"url":"https://example.invalid/mcp"}`, `{"command":"custom-wrapper","args":["mcp"]}`} {
		path := filepath.Join(home, ".claude.json")
		original := []byte(`{"mcpServers":{"deptrust":` + server + `}}`)
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
		if err := refreshClaudeMCP(path, "/new/deptrust"); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, original) {
			t.Fatal("custom or remote server changed")
		}
	}
}

func TestRefreshPreservesClaudeHookOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeConfig(t, path, map[string]any{"env": map[string]string{"DEPTRUST_GITHUB_AUTH": "gh"}, "hooks": map[string]any{"PreToolUse": []any{map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": "/old/deptrust", "args": []string{"hook", "tool"}, "timeout": 60}}}}}})
	if err := refreshHookPaths(path, "/new/deptrust"); err != nil {
		t.Fatal(err)
	}
	config := readConfig(t, path)
	b, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"command":"/new/deptrust"`) || !strings.Contains(string(b), `"args":["hook","tool"]`) || !strings.Contains(string(b), `"timeout":60`) || !strings.Contains(string(b), `"DEPTRUST_GITHUB_AUTH":"gh"`) {
		t.Fatal(string(b))
	}
}

func TestStableHomebrewPathSurvivesUpgrade(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Homebrew symlink test")
	}
	prefix := t.TempDir()
	old := filepath.Join(prefix, "Cellar", "deptrust", "1.0.0", "bin", "deptrust")
	newBinary := filepath.Join(prefix, "Cellar", "deptrust", "2.0.0", "bin", "deptrust")
	for _, path := range []string{old, newBinary} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(path), 0700); err != nil {
			t.Fatal(err)
		}
	}
	opt := filepath.Join(prefix, "opt", "deptrust")
	if err := os.MkdirAll(filepath.Dir(opt), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(filepath.Dir(old)), opt); err != nil {
		t.Fatal(err)
	}
	stable := stableSetupExecutable(old)
	want := filepath.Join(opt, "bin", "deptrust")
	if stable != want {
		t.Fatalf("got %s, want %s", stable, want)
	}
	if err := os.Remove(opt); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(filepath.Dir(newBinary)), opt); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(stable)
	if err != nil || string(b) != newBinary {
		t.Fatal("stable path did not follow upgrade")
	}
	if got := stableSetupExecutable(old); got != old {
		t.Fatalf("selected a different binary: %s", got)
	}
}

func TestRefreshLeavesUnrelatedHookArgumentsAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	original := []byte(`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"other-tool","args":["hook","shell"]},{"type":"command","command":"other-deptrust-wrapper hook shell"}]}]}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := refreshHookPaths(path, "/new/deptrust"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatal("unrelated hooks changed")
	}
}

func TestRefreshRejectsInvalidConfigurationWithoutRewriting(t *testing.T) {
	for _, source := range []string{`{"mcpServers":`, `{} {}`} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		if err := refreshClaudeMCP(path, "/new/deptrust"); err == nil {
			t.Fatal("accepted invalid configuration")
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != source {
			t.Fatal("rewrote invalid configuration")
		}
	}
}
