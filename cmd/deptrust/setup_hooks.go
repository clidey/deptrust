package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const (
	codexHookMatcher  = "Bash|apply_patch|Edit|Write|MultiEdit"
	claudeHookMatcher = "Bash|Edit|Write|MultiEdit"
)

func configureHooks(executable string, install bool, stdout io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory for hooks: %w", err)
	}

	targets := []hookTarget{
		{path: filepath.Join(home, ".codex", "hooks.json"), matcher: codexHookMatcher, command: shellQuote(executable) + " hook shell", statusMessage: "Checking dependency safety with deptrust"},
		{path: filepath.Join(home, ".claude", "settings.json"), matcher: claudeHookMatcher, command: executable, args: []string{"hook", "shell"}},
	}
	for _, target := range targets {
		present, err := deptrustHookPresent(target.path)
		if err != nil {
			return err
		}
		if !install && !present {
			continue
		}
		changed, err := reconcileHook(target)
		if err != nil {
			return err
		}
		if changed {
			fmt.Fprintf(stdout, "Updated the deptrust hook in %s.\n", target.path)
		} else {
			fmt.Fprintf(stdout, "deptrust hook is already configured in %s.\n", target.path)
		}
	}
	return nil
}

type hookTarget struct {
	path          string
	matcher       string
	command       string
	args          []string
	statusMessage string
}

func reconcileHook(target hookTarget) (bool, error) {
	config := map[string]any{}
	contents, err := os.ReadFile(target.path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read hook config %s: %w", target.path, err)
	}
	if len(contents) > 0 {
		if err := json.Unmarshal(contents, &config); err != nil {
			return false, fmt.Errorf("parse hook config %s: %w", target.path, err)
		}
	}

	hooks, err := object(config, "hooks")
	if err != nil {
		return false, fmt.Errorf("read hook config %s: %w", target.path, err)
	}
	preToolUse, err := array(hooks, "PreToolUse")
	if err != nil {
		return false, fmt.Errorf("read hook config %s: %w", target.path, err)
	}
	filtered := removeDeptrustHooks(preToolUse)
	filtered = append(filtered, map[string]any{
		"matcher": target.matcher,
		"hooks":   []any{hookEntry(target)},
	})
	if reflect.DeepEqual(preToolUse, filtered) {
		return false, nil
	}
	hooks["PreToolUse"] = filtered
	config["hooks"] = hooks

	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encode hook config %s: %w", target.path, err)
	}
	encoded = append(encoded, '\n')
	if err := os.MkdirAll(filepath.Dir(target.path), 0700); err != nil {
		return false, fmt.Errorf("create hook config directory %s: %w", filepath.Dir(target.path), err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(target.path), ".deptrust-*.json")
	if err != nil {
		return false, fmt.Errorf("create hook config %s: %w", target.path, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(encoded); err != nil {
		temporary.Close()
		return false, fmt.Errorf("write hook config %s: %w", target.path, err)
	}
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return false, fmt.Errorf("set hook config permissions %s: %w", target.path, err)
	}
	if err := temporary.Close(); err != nil {
		return false, fmt.Errorf("close hook config %s: %w", target.path, err)
	}
	if err := os.Rename(temporaryPath, target.path); err != nil {
		return false, fmt.Errorf("replace hook config %s: %w", target.path, err)
	}
	return true, nil
}

func deptrustHookPresent(path string) (bool, error) {
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read hook config %s: %w", path, err)
	}
	config := map[string]any{}
	if err := json.Unmarshal(contents, &config); err != nil {
		return false, fmt.Errorf("parse hook config %s: %w", path, err)
	}
	hooks, err := object(config, "hooks")
	if err != nil {
		return false, fmt.Errorf("read hook config %s: %w", path, err)
	}
	preToolUse, err := array(hooks, "PreToolUse")
	if err != nil {
		return false, fmt.Errorf("read hook config %s: %w", path, err)
	}
	for _, group := range preToolUse {
		entry, ok := group.(map[string]any)
		if !ok {
			continue
		}
		entries, err := array(entry, "hooks")
		if err != nil {
			continue
		}
		for _, hook := range entries {
			if isDeptrustHook(hook) {
				return true, nil
			}
		}
	}
	return false, nil
}

func hookEntry(target hookTarget) map[string]any {
	entry := map[string]any{"type": "command", "command": target.command}
	if len(target.args) > 0 {
		args := make([]any, len(target.args))
		for index, arg := range target.args {
			args[index] = arg
		}
		entry["args"] = args
	}
	if target.statusMessage != "" {
		entry["statusMessage"] = target.statusMessage
	}
	return entry
}

func object(config map[string]any, key string) (map[string]any, error) {
	value, ok := config[key]
	if !ok {
		return map[string]any{}, nil
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%q must be an object", key)
	}
	return result, nil
}

func array(config map[string]any, key string) ([]any, error) {
	value, ok := config[key]
	if !ok {
		return []any{}, nil
	}
	result, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%q must be an array", key)
	}
	return result, nil
}

func removeDeptrustHooks(groups []any) []any {
	filtered := make([]any, 0, len(groups))
	for _, value := range groups {
		group, ok := value.(map[string]any)
		if !ok {
			filtered = append(filtered, value)
			continue
		}
		hooks, err := array(group, "hooks")
		if err != nil {
			filtered = append(filtered, value)
			continue
		}
		kept := make([]any, 0, len(hooks))
		for _, hook := range hooks {
			if !isDeptrustHook(hook) {
				kept = append(kept, hook)
			}
		}
		if len(kept) > 0 {
			group["hooks"] = kept
			filtered = append(filtered, group)
		}
	}
	return filtered
}

func isDeptrustHook(value any) bool {
	hook, ok := value.(map[string]any)
	if !ok || hook["type"] != "command" {
		return false
	}
	if args, ok := hook["args"].([]any); ok && len(args) >= 2 && args[0] == "hook" && (args[1] == "shell" || args[1] == "tool") {
		return true
	}
	command, ok := hook["command"].(string)
	if !ok {
		return false
	}
	fields := strings.Fields(command)
	if len(fields) == 1 && filepath.Base(fields[0]) == "deptrust" {
		return true // Legacy hook entries omitted the required "hook shell" arguments.
	}
	return strings.Contains(command, "deptrust") && (strings.Contains(command, "hook shell") || strings.Contains(command, "hook tool"))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
