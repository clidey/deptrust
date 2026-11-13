package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/clidey/deptrust/internal/app"
)

func TestSimpleInstallLiteralRequests(t *testing.T) {
	for _, tc := range []struct {
		command string
		want    []packageRequest
	}{
		{"npm install lodash@4.17.21 axios@1.0.0", []packageRequest{{"npm", "lodash", "4.17.21"}, {"npm", "axios", "1.0.0"}}},
		{"pnpm add @scope/pkg@1.2.3", []packageRequest{{"npm", "@scope/pkg", "1.2.3"}}},
		{"yarn add lodash", []packageRequest{{"npm", "lodash", "latest"}}},
		{"bun add lodash@latest", []packageRequest{{"npm", "lodash", "latest"}}},
		{"pip3 install requests==2.32.3", []packageRequest{{"pypi", "requests", "2.32.3"}}},
		{"uv pip install requests==2.32.3", []packageRequest{{"pypi", "requests", "2.32.3"}}},
		{"uv add requests==2.32.3", []packageRequest{{"pypi", "requests", "2.32.3"}}},
		{"cargo add serde@1.0.0", []packageRequest{{"cargo", "serde", "1.0.0"}}},
		{"go get example.com/mod/v2@v2.0.0", []packageRequest{{"go", "example.com/mod/v2", "v2.0.0"}}},
		{"bundle add rack --version 3.0.0", []packageRequest{{"rubygems", "rack", "3.0.0"}}},
		{"gem install rack -v 3.0.0", []packageRequest{{"rubygems", "rack", "3.0.0"}}},
		{"dotnet add package Example --version 1.0.0", []packageRequest{{"nuget", "Example", "1.0.0"}}},
		{"composer require vendor/package:1.0.0", []packageRequest{{"packagist", "vendor/package", "1.0.0"}}},
		{"mvn dependency:get -Dartifact=org.example:library:1.0.0", []packageRequest{{"maven", "org.example:library", "1.0.0"}}},
		{"pnpm install", nil}, // No explicit package requests; manifest auditing is separate.
		{"yarn install", nil},
		{"bundle install", nil},
		{"composer install", nil},
		{"npm test", nil},
		{"go test ./...", nil},
		{"git status && git diff", nil},
	} {
		t.Run(tc.command, func(t *testing.T) {
			got, err := ParseInstallCommand(tc.command)
			if err != nil {
				t.Fatal(err)
			}
			assertRequests(t, got, tc.want)
		})
	}
}

func TestSimpleInstallRetriesBeforeCheckingPackages(t *testing.T) {
	for _, command := range []string{
		"pnpm install 2>&1", "pnpm add lodash 2>&1", "pnpm add lodash>out.log",
		"pnpm >out.log add lodash", "pnpm add lodash 2>&1 axios", "|& pnpm add lodash",
		"pnpm add lodash && npm install axios", "cd project && pnpm add lodash",
		"pnpm add lodash; npm install axios", "pnpm add lodash\nnpm install axios",
		"pnpm add lodash\r\n", "pnpm add lodash\x00", "pnpm add lodash # comment",
		"pnpm add $PACKAGE", "pnpm add ${PACKAGE}", "pnpm add $(echo lodash)",
		"pnpm add `echo lodash`", "pnpm add %PACKAGE%", "pnpm add !PACKAGE!",
		`pnpm add "lodash"`, "pnpm add 'lodash'", `pnpm add "unterminated`,
		"pnpm add lo^dash", `pnpm add lo\dash`, "pnpm add lodash@^4", "pnpm add lodash@*",
		"pnpm add ./local", "npm install /tmp/local", "npm install https://example.com/pkg.tgz",
		"npm install alias@npm:lodash@4.17.21", "npm install lodash@", "pip install requests==",
		"composer require vendor/pkg:", "mvn dependency:get -Dartifact=org.example:lib:",
		"npm install --save-dev lodash", "npm install --registry=https://private.invalid lodash",
		"cargo add serde --features derive", "pip install -r requirements.txt",
		"gem install rack puma", "bundle add rack --version", "dotnet add package Example --version",
		"mvn -Pprivate dependency:get -Dartifact=company:secret:1.0.0",
		"python -m pip install requests", "env npm install lodash", "npm.cmd install lodash",
		"powershell -Command pnpm add lodash", "cmd /c npm install lodash",
		"pnpm add " + strings.Repeat("a", 65536),
	} {
		t.Run(command[:min(len(command), 80)], func(t *testing.T) {
			got, err := ParseInstallCommand(command)
			if err == nil || len(got) != 0 {
				t.Fatalf("got %#v, %v; want retry without partial requests", got, err)
			}
			input, err := json.Marshal(map[string]any{"tool_input": map[string]string{"command": command}})
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			// A zero App must never be called: rejection happens before lookups.
			if err := RunShell(context.Background(), app.App{}, bytes.NewReader(input), &out); err != nil {
				t.Fatal(err)
			}
			var response claudeHookOutput
			if err := json.Unmarshal(out.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			decision := response.HookSpecificOutput
			if decision.PermissionDecision != "deny" || !strings.Contains(decision.PermissionDecisionReason, "Retry") {
				t.Fatal(out.String())
			}
		})
	}
}

func FuzzSimpleInstallCommand(f *testing.F) {
	for _, s := range []string{"pnpm add lodash@4.17.21", "pnpm install 2>&1", "|&", "pnpm add $PACKAGE", "go get example.com/mod@v1.0.0", "npm install --registry=private lodash"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, command string) {
		requests, err := ParseInstallCommand(command)
		if err != nil && len(requests) != 0 {
			t.Fatal("partial requests on error")
		}
		for _, r := range requests {
			if !simplePackageName(r.Ecosystem, r.Name) || !literalVersionRE.MatchString(r.Version) {
				t.Fatalf("nonliteral request: %#v", r)
			}
		}
	})
}
