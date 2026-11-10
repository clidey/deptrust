package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clidey/deptrust/internal/models"
	"github.com/clidey/deptrust/internal/sourcetest"
)

// TestSourceProcesses exercises shipped entrypoints, subprocess source readers,
// MCP follow-up commands, and an HTTP tripwire rather than in-process mocks.
func TestSourceProcesses(t *testing.T) {
	if os.Getenv("DEPTRUST_TEST_E2E") != "1" {
		t.Skip("set DEPTRUST_TEST_E2E=1 with required toolchains")
	}
	root := t.TempDir()
	binary := filepath.Join(root, "deptrust")
	build := exec.Command("go", "build", "-race", "-o", binary, "./cmd/deptrust")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(502) }))
	defer server.Close()
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, server.URL)
	}
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	fixtures := filepath.Join(root, "source fixtures' quoted")
	if err := os.Mkdir(fixtures, 0700); err != nil {
		t.Fatal(err)
	}
	sourcetest.Elixir(t, fixtures)
	write := func(dir, name, body string) string {
		t.Helper()
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			data, err := os.ReadFile(file)
			if err != nil || string(data) != body {
				t.Errorf("fixture modified: %s", file)
			}
		})
		return file
	}
	nuget := filepath.Join(fixtures, "nuget")
	write(nuget, "NuGet.Config", `<configuration><packageSources><clear/><add key="internal" value="`+server.URL+`"/></packageSources></configuration>`)
	maven := filepath.Join(fixtures, "maven")
	write(maven, "pom.xml", `<project><modelVersion>4.0.0</modelVersion><groupId>fixture</groupId><artifactId>test</artifactId><version>1</version><dependencies><dependency><groupId>company</groupId><artifactId>secret</artifactId><version>1</version></dependency></dependencies><repositories><repository><id>internal</id><url>`+server.URL+`</url></repository></repositories></project>`)
	settings := write(root, "settings.xml", `<settings/>`)
	t.Setenv("MAVEN_ARGS", "-s "+settings)
	t.Setenv("MAVEN_SKIP_RC", "1")
	gradle := filepath.Join(fixtures, "gradle")
	write(gradle, "settings.gradle", "rootProject.name='source-test'\n")
	write(gradle, "build.gradle", "configurations { inspection }\nrepositories { maven { url = uri('"+server.URL+"'); allowInsecureProtocol = true } }\ndependencies { inspection 'company:secret:1' }\ntasks.register('ordinaryBuild') { doLast { throw new GradleException('ordinary build ran') } }\n")
	t.Setenv("GRADLE_USER_HOME", filepath.Join(root, "gradle-home"))
	mix := write(fixtures, "mix.lock", `%{"secret" => {:hex, :secret, "1.0.0", "hash", [:mix], [], "hexpm:company", "hash"}}`)
	npm := filepath.Join(fixtures, "npm")
	write(npm, ".npmrc", "@company:registry="+server.URL+"\n")
	cargo := filepath.Join(fixtures, "cargo")
	write(cargo, "Cargo.toml", "[package]\nname='fixture'\nversion='0.1.0'\n[dependencies]\nsecret={path='../secret'}\n")
	write(filepath.Join(cargo, "src"), "lib.rs", "")
	write(filepath.Join(fixtures, "secret"), "Cargo.toml", "[package]\nname='secret'\nversion='0.1.0'\n")
	write(filepath.Join(fixtures, "secret/src"), "lib.rs", "")
	bundler := write(fixtures, "Gemfile.lock", "GEM\n  remote: https://user:credential-sentinel@private.invalid/\n  specs:\n    secret (1.0.0)\n")
	t.Setenv("GOPRIVATE", "company.invalid")
	queries := sourcetest.ArtifactQueries(t)
	queries = append(queries,
		models.Query{Ecosystem: models.EcosystemNPM, Package: "@company/secret", ProjectDir: npm},
		models.Query{Ecosystem: models.EcosystemCargo, Package: "secret", ProjectDir: cargo},
		models.Query{Ecosystem: models.EcosystemRuby, Package: "secret", BundlerLock: bundler},
		models.Query{Ecosystem: models.EcosystemNuGet, Package: "Company.Secret", ProjectDir: nuget},
		models.Query{Ecosystem: models.EcosystemMaven, Package: "company:secret", ProjectDir: maven},
		models.Query{Ecosystem: models.EcosystemMaven, Package: "company:secret", ProjectDir: gradle},
		models.Query{Ecosystem: models.EcosystemHex, Package: "secret", SourceFile: mix},
	)
	run := func(t *testing.T, input []byte, args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Stdin = bytes.NewReader(input)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%v: %v %s %s", args, err, out, stderr.String())
		}
		if strings.Contains(string(out)+stderr.String(), "credential-sentinel") {
			t.Fatal("credential leaked")
		}
		return out
	}
	unknown := func(t *testing.T, out []byte) map[string]any {
		t.Helper()
		var data map[string]any
		if err := json.Unmarshal(out, &data); err != nil {
			t.Fatalf("invalid JSON: %v %s", err, out)
		}
		if data["recommendation"] != "unknown" || data["safe_to_use"] == true || data["should_install"] == true || data["improves_risk"] == true {
			t.Fatalf("unsafe assessment: %s", out)
		}
		return data
	}
	t.Run("go/GOPRIVATE", func(t *testing.T) {
		unknown(t, run(t, nil, "check", "--json", "go", "company.invalid/secret", "v1.0.0"))
	})
	for _, command := range []string{`mvn "-Pprivate" dependency:get -Dartifact=company:secret:1.0.0`, `npm install --reg"is"try=https://private.invalid secret@1.0.0`} {
		request, err := json.Marshal(map[string]any{"tool_input": map[string]string{"command": command}})
		if err != nil {
			t.Fatal(err)
		}
		out := run(t, request, "hook", "shell")
		if !bytes.Contains(out, []byte(`"permissionDecision":"deny"`)) {
			t.Fatalf("override not denied: %s", out)
		}
	}
	for _, q := range queries {
		t.Run(string(q.Ecosystem)+"/"+filepath.Base(q.ProjectDir+q.SourceFile+q.NuGetConfig+q.BundlerLock), func(t *testing.T) {
			flag, value, key := "--source-file", q.SourceFile, "source_file"
			if q.ProjectDir != "" {
				flag, value, key = "--project-dir", q.ProjectDir, "project_dir"
			}
			if q.NuGetConfig != "" {
				flag, value, key = "--nuget-config", q.NuGetConfig, "nuget_config"
			}
			if q.BundlerLock != "" {
				flag, value, key = "--bundler-lock", q.BundlerLock, "bundler_lock"
			}
			operations := []string{"check", "suggest", "compare"}
			for _, op := range operations {
				versions := []string{"1.0.0"}
				method := "check_package"
				if op == "suggest" {
					versions = nil
					method = "suggest_safe_version"
				}
				if op == "compare" {
					versions = []string{"1.0.0", "2.0.0"}
					method = "compare_versions"
				}
				args := []string{op, "--json", flag, value, string(q.Ecosystem), q.Package}
				unknown(t, run(t, nil, append(args, versions...)...))
				arguments := map[string]any{"ecosystem": q.Ecosystem, "package": q.Package, "version": "1.0.0", "from_version": "1.0.0", "to_version": "2.0.0", key: value}
				request, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": method, "arguments": arguments}})
				if err != nil {
					t.Fatal(err)
				}
				out := run(t, append(request, '\n'), "mcp")
				var response struct {
					Result struct {
						Error bool            `json:"isError"`
						Data  json.RawMessage `json:"structuredContent"`
					}
				}
				if err := json.Unmarshal(out, &response); err != nil || response.Result.Error {
					t.Fatalf("MCP: %v %s", err, out)
				}
				data := unknown(t, response.Result.Data)
				command, ok := data["full_response_command"].(string)
				if !ok || !strings.Contains(command, flag) {
					t.Fatalf("context lost: %s", out)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				replay := exec.CommandContext(ctx, "sh", "-c", command)
				replayed, err := replay.CombinedOutput()
				cancel()
				if err != nil {
					t.Fatalf("follow-up: %v %s", err, replayed)
				}
				unknown(t, replayed)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("%d HTTP requests escaped source checks", calls.Load())
	}
	if _, err := os.Stat(filepath.Join(maven, "target")); !os.IsNotExist(err) {
		t.Fatal("Maven lifecycle ran")
	}
}
