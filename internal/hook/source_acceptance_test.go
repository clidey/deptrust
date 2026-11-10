package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clidey/deptrust/internal/app"
	"github.com/clidey/deptrust/internal/sourcetest"
)

func TestHookPreservesNPMSourceContext(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm required")
	}
	sourcetest.Guard(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".npmrc"), []byte("@company:registry=https://private.invalid/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"cwd": dir, "tool_input": map[string]string{"command": "npm install @company/secret@1.0.0"}}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunShell(context.Background(), app.New(), bytes.NewReader(encoded), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"deny"`) || !strings.Contains(out.String(), "unknown") {
		t.Fatal(out.String())
	}
}

func TestHookSourceOverridesStopBeforeLookup(t *testing.T) {
	sourcetest.Guard(t)
	for _, command := range []string{"npm install --registry=https://private.invalid secret@1.0.0", "pip install --index-url https://private.invalid secret==1.0.0"} {
		encoded, err := json.Marshal(map[string]any{"tool_input": map[string]string{"command": command}})
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := RunShell(context.Background(), app.New(), bytes.NewReader(encoded), &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), `"deny"`) {
			t.Fatal(out.String())
		}
	}
}

func TestHookExplicitSourceContext(t *testing.T) {
	sourcetest.Guard(t)
	input := map[string]any{"source_context": map[string]string{"nuget_config": sourcetest.NuGetConfig(t)}, "tool_input": map[string]string{"command": "dotnet add package Company.Secret --version 1.0.0"}}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunShell(context.Background(), app.New(), bytes.NewReader(encoded), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"deny"`) || !strings.Contains(out.String(), "unknown") {
		t.Fatal(out.String())
	}
	if strings.Contains(out.String(), "credential-sentinel") {
		t.Fatal("credential disclosed")
	}
}

func TestHookCannotGuessInheritedSources(t *testing.T) {
	sourcetest.Guard(t)
	input := map[string]any{"cwd": t.TempDir(), "tool_input": map[string]string{"command": "pip install secret==1.0.0"}}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunShell(context.Background(), app.New(), bytes.NewReader(encoded), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"deny"`) || !strings.Contains(out.String(), "source_context") {
		t.Fatal(out.String())
	}
}
