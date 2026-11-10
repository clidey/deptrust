package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/clidey/deptrust/internal/sourcetest"
)

func TestSourcePrivacyCLI(t *testing.T) {
	sourcetest.Guard(t)
	file := sourcetest.NuGetConfig(t)
	for _, tt := range []struct {
		command  string
		versions []string
	}{
		{"check", []string{"1.0.0"}}, {"check", []string{"latest"}}, {"suggest", nil}, {"compare", []string{"1.0.0", "2.0.0"}},
	} {
		args := []string{tt.command, "--json", "--nuget-config", file, "nuget", "Company.Secret"}
		args = append(args, tt.versions...)
		var out, stderr bytes.Buffer
		if err := run(args, &out, &stderr); err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		if err := json.Unmarshal(out.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if data["recommendation"] != "unknown" || data["safe_to_use"] == true || data["improves_risk"] == true {
			t.Fatalf("%s", out.String())
		}
		if strings.Contains(out.String(), "credential-sentinel") {
			t.Fatal("credential disclosed")
		}
	}
}
