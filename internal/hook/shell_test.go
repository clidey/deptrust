package hook

import (
	"encoding/json"
	"testing"
)

func TestParseInstallCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want []packageRequest
	}{
		{
			name: "pnpm scoped package",
			cmd:  "pnpm add @clidey/ux@1.2.3",
			want: []packageRequest{{Ecosystem: "npm", Name: "@clidey/ux", Version: "1.2.3"}},
		},
		{
			name: "npm latest",
			cmd:  "npm install lodash",
			want: []packageRequest{{Ecosystem: "npm", Name: "lodash", Version: "latest"}},
		},
		{
			name: "pip exact",
			cmd:  "pip install requests==2.32.3",
			want: []packageRequest{{Ecosystem: "pypi", Name: "requests", Version: "2.32.3"}},
		},
		{
			name: "go get",
			cmd:  "go get golang.org/x/crypto@v0.31.0",
			want: []packageRequest{{Ecosystem: "go", Name: "golang.org/x/crypto", Version: "v0.31.0"}},
		},
		{
			name: "dotnet package",
			cmd:  "dotnet add package Newtonsoft.Json --version 13.0.3",
			want: []packageRequest{{Ecosystem: "nuget", Name: "Newtonsoft.Json", Version: "13.0.3"}},
		},
		{
			name: "maven artifact",
			cmd:  "mvn dependency:get -Dartifact=com.google.guava:guava:33.0.0",
			want: []packageRequest{{Ecosystem: "maven", Name: "com.google.guava:guava", Version: "33.0.0"}},
		},
		{
			name: "ignore test command",
			cmd:  "npm test",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseInstallCommand(tt.cmd)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseInstallCommand() = %#v, want %#v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ParseInstallCommand()[%d] = %#v, want %#v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseGitHubActionsUses(t *testing.T) {
	text := `
jobs:
  test:
    steps:
      - uses: actions/checkout@v4
      - uses: github/codeql-action/upload-sarif@v3.28.10
      - uses: ./local-action
      - uses: docker://alpine:3
`
	want := []packageRequest{
		{Ecosystem: "github-actions", Name: "actions/checkout", Version: "v4"},
		{Ecosystem: "github-actions", Name: "github/codeql-action", Version: "v3.28.10"},
	}
	got := ParseGitHubActionsUses(text, false)
	assertRequests(t, got, want)
}

func TestParseGitHubActionsUsesPatchAddedLinesOnly(t *testing.T) {
	patch := `
@@
-      - uses: actions/checkout@v3
+      - uses: actions/checkout@v4
       - run: go test ./...
`
	want := []packageRequest{{Ecosystem: "github-actions", Name: "actions/checkout", Version: "v4"}}
	got := ParseGitHubActionsUses(patch, true)
	assertRequests(t, got, want)
}

func TestParseHookInputFromWrite(t *testing.T) {
	input := hookInput{
		ToolName: "Write",
		ToolInput: map[string]json.RawMessage{
			"file_path": json.RawMessage(`".github/workflows/ci.yml"`),
			"content":   json.RawMessage(`"jobs:\n  test:\n    steps:\n      - uses: actions/setup-go@v6.1.0\n"`),
		},
	}
	want := []packageRequest{{Ecosystem: "github-actions", Name: "actions/setup-go", Version: "v6.1.0"}}
	got := ParseHookInput(input)
	assertRequests(t, got, want)
}

func TestParseHookInputDedupesCommandAndWorkflow(t *testing.T) {
	input := hookInput{
		ToolName: "Bash",
		ToolInput: map[string]json.RawMessage{
			"command": json.RawMessage(`"npm install lodash@4.17.21"`),
			"patch":   json.RawMessage(`"+      - uses: actions/checkout@v4\n+      - uses: actions/checkout@v4\n"`),
		},
	}
	want := []packageRequest{
		{Ecosystem: "npm", Name: "lodash", Version: "4.17.21"},
		{Ecosystem: "github-actions", Name: "actions/checkout", Version: "v4"},
	}
	got := ParseHookInput(input)
	assertRequests(t, got, want)
}

func assertRequests(t *testing.T, got, want []packageRequest) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("requests = %#v, want %#v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("requests[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
