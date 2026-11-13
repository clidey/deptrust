package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// Homebrew's opt symlink survives Cellar version changes. Only select it when
// it resolves to this binary, so an unrelated installation cannot be selected.
func stableSetupExecutable(executable string) string {
	path := filepath.ToSlash(executable)
	prefix, _, ok := strings.Cut(path, "/Cellar/deptrust/")
	if !ok {
		return executable
	}
	candidate := filepath.Join(filepath.FromSlash(prefix), "opt", "deptrust", "bin", filepath.Base(executable))
	current, err := os.Stat(executable)
	if err != nil {
		return executable
	}
	target, err := os.Stat(candidate)
	if err == nil && os.SameFile(current, target) {
		return candidate
	}
	return executable
}

// Refresh only existing local user registrations. No agent process, profile,
// project configuration or remote MCP connection needs to run during an upgrade.
func refreshIntegrations(executable string, stdout io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := refreshCodexMCP(filepath.Join(home, ".codex", "config.toml"), executable); err != nil {
		return err
	}
	if err := refreshClaudeMCP(filepath.Join(home, ".claude.json"), executable); err != nil {
		return err
	}
	for _, path := range []string{filepath.Join(home, ".codex", "hooks.json"), filepath.Join(home, ".claude", "settings.json")} {
		if err := refreshHookPaths(path, executable); err != nil {
			return err
		}
	}
	if err := configurePackageCheckSkill(home, false, stdout); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Existing deptrust integrations are up to date.")
	return nil
}

func localDeptrustMCP(entry map[string]any) bool {
	command, _ := entry["command"].(string)
	args, _ := entry["args"].([]any)
	return oneOfBinary(command) && len(args) > 0 && args[0] == "mcp" && entry["url"] == nil
}

func oneOfBinary(command string) bool {
	name := filepath.Base(command)
	return name == "deptrust" || name == "deptrust.exe"
}

func refreshCodexMCP(path, executable string) error {
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var config map[string]any
	if err := toml.Unmarshal(contents, &config); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	servers, _ := config["mcp_servers"].(map[string]any)
	entry, _ := servers["deptrust"].(map[string]any)
	if !localDeptrustMCP(entry) || entry["command"] == executable {
		return nil
	}
	// Replace only the string node, preserving comments, formatting, environment,
	// timeouts and all unrelated TOML configuration.
	var parser unstable.Parser
	parser.Reset(contents)
	var table []string
	var commandRange *unstable.Range
	var visit func(*unstable.Node, []string)
	visit = func(n *unstable.Node, prefix []string) {
		key := append([]string{}, prefix...)
		for it := n.Key(); it.Next(); {
			key = append(key, string(it.Node().Data))
		}
		value := n.Value()
		if slices.Equal(key, []string{"mcp_servers", "deptrust", "command"}) && value.Kind == unstable.String {
			r := value.Raw
			commandRange = &r
		}
		if value.Kind == unstable.InlineTable {
			for it := value.Children(); it.Next(); {
				visit(it.Node(), key)
			}
		}
	}
	for parser.NextExpression() {
		n := parser.Expression()
		switch n.Kind {
		case unstable.Table, unstable.ArrayTable:
			table = nil
			for it := n.Key(); it.Next(); {
				table = append(table, string(it.Node().Data))
			}
		case unstable.KeyValue:
			visit(n, table)
		}
	}
	if err := parser.Error(); err != nil {
		return err
	}
	if commandRange == nil {
		return fmt.Errorf("cannot locate deptrust command in %s; run deptrust setup", path)
	}
	encoded, err := toml.Marshal(map[string]string{"command": executable})
	if err != nil {
		return err
	}
	_, literal, _ := bytes.Cut(encoded, []byte("="))
	r := *commandRange
	updated := append([]byte{}, contents[:r.Offset]...)
	updated = append(updated, bytes.TrimSpace(literal)...)
	updated = append(updated, contents[r.Offset+r.Length:]...)
	// Validate the edited document before replacing the existing config.
	var checked map[string]any
	if err := toml.Unmarshal(updated, &checked); err != nil {
		return fmt.Errorf("update %s: %w", path, err)
	}
	return replaceSetupFile(path, updated)
}

func refreshClaudeMCP(path, executable string) error {
	return refreshJSON(path, func(config map[string]any) bool {
		servers, _ := config["mcpServers"].(map[string]any)
		entry, _ := servers["deptrust"].(map[string]any)
		if !localDeptrustMCP(entry) || entry["command"] == executable {
			return false
		}
		entry["command"] = executable
		return true
	})
}

func refreshHookPaths(path, executable string) error {
	return refreshJSON(path, func(config map[string]any) bool {
		hooks, _ := config["hooks"].(map[string]any)
		groups, _ := hooks["PreToolUse"].([]any)
		changed := false
		for _, group := range groups {
			g, _ := group.(map[string]any)
			entries, _ := g["hooks"].([]any)
			for _, value := range entries {
				entry, ok := value.(map[string]any)
				if !ok || entry["type"] != "command" {
					continue
				}
				old, _ := entry["command"].(string)
				command := executable
				if args, ok := entry["args"].([]any); ok && len(args) > 0 {
					if !oneOfBinary(old) || len(args) < 2 || args[0] != "hook" || (args[1] != "shell" && args[1] != "tool") {
						continue
					}
				} else {
					prefix := ""
					if strings.HasPrefix(old, "DEPTRUST_GITHUB_AUTH=gh ") {
						prefix = "DEPTRUST_GITHUB_AUTH=gh "
					}
					binary := strings.TrimPrefix(old, prefix)
					suffix := " hook shell"
					for _, candidate := range []string{" hook shell", " hook tool"} {
						if strings.HasSuffix(binary, candidate) {
							binary = strings.TrimSuffix(binary, candidate)
							suffix = candidate
							break
						}
					}
					if !oneOfBinary(strings.Trim(binary, "'\"")) {
						continue
					}
					command = prefix + shellQuote(executable) + suffix
				}
				if old != command {
					entry["command"] = command
					changed = true
				}
			}
		}
		return changed
	})
}

func refreshJSON(path string, update func(map[string]any) bool) error {
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var config map[string]any
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	if err := decoder.Decode(&config); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("read %s: expected one JSON document", path)
	}
	if !update(config) {
		return nil
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return replaceSetupFile(path, append(encoded, '\n'))
}

func replaceSetupFile(path string, contents []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".deptrust-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(contents); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(info.Mode().Perm()); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
