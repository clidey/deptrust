package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/clidey/deptrust/internal/app"
	"github.com/clidey/deptrust/internal/sourcetest"
)

func TestSourcePrivacyProtocol(t *testing.T) {
	sourcetest.Guard(t)
	file := sourcetest.NuGetConfig(t)
	service := app.New()
	for _, name := range []string{"check_package", "suggest_safe_version", "compare_versions"} {
		args := map[string]any{"ecosystem": "nuget", "package": "Company.Secret", "version": "1.0.0", "from_version": "1.0.0", "to_version": "2.0.0", "nuget_config": file}
		request := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}}
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := Serve(context.Background(), service, bytes.NewReader(append(encoded, '\n')), &out); err != nil {
			t.Fatal(err)
		}
		var response struct {
			Result struct {
				IsError bool           `json:"isError"`
				Data    map[string]any `json:"structuredContent"`
			}
		}
		if err := json.Unmarshal(out.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		data := response.Result.Data
		if response.Result.IsError || data["recommendation"] != "unknown" {
			t.Fatalf("%s", out.String())
		}
		command, ok := data["full_response_command"].(string)
		if !ok || !strings.Contains(command, "--nuget-config "+shellQuote(file)) {
			t.Fatalf("context lost: %s", command)
		}
		if strings.Contains(out.String(), "credential-sentinel") {
			t.Fatal("credential disclosed")
		}
	}
}
