package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestNuGetInheritedConfiguration(t *testing.T) {
	if _, err := exec.LookPath("dotnet"); err != nil {
		t.Skip(".NET SDK required")
	}
	for _, tt := range []struct {
		name, child string
		blocked     bool
	}{
		{"parent private mapping", "<configuration/>", true},
		{"child clears mappings", `<configuration><packageSourceMapping><clear/><packageSource key="public"><package pattern="*"/></packageSource></packageSourceMapping></configuration>`, false},
		{"disabled private cannot fallback", `<configuration><disabledPackageSources><add key="private" value="true"/></disabledPackageSources></configuration>`, true},
		{"removed private cannot fallback", `<configuration><packageSources><remove key="private"/></packageSources></configuration>`, true},
		{"source pattern collections replace", `<configuration><packageSourceMapping><packageSource key="private"><package pattern="Other.*"/></packageSource></packageSourceMapping></configuration>`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			project := filepath.Join(root, "child")
			if err := os.Mkdir(project, 0700); err != nil {
				t.Fatal(err)
			}
			parent := `<configuration><packageSources><clear/><add key="public" value="https://api.nuget.org/v3/index.json"/><add key="private" value="https://private.invalid"/></packageSources><disabledPackageSources><clear/></disabledPackageSources><packageSourceMapping><clear/><packageSource key="public"><package pattern="*"/></packageSource><packageSource key="private"><package pattern="Company.*"/></packageSource></packageSourceMapping></configuration>`
			for file, body := range map[string]string{filepath.Join(root, "NuGet.Config"): parent, filepath.Join(project, "NuGet.Config"): tt.child} {
				if err := os.WriteFile(file, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			q := models.Query{Ecosystem: models.EcosystemNuGet, Package: "Company.Secret", ProjectDir: project}
			reason, err := sourceRestriction(context.Background(), q)
			if err != nil || (reason != "") != tt.blocked {
				t.Fatalf("restriction=%q error=%v", reason, err)
			}
			if tt.blocked {
				assertUnavailableSource(t, q)
			}
			for file, body := range map[string]string{filepath.Join(root, "NuGet.Config"): parent, filepath.Join(project, "NuGet.Config"): tt.child} {
				data, err := os.ReadFile(file)
				if err != nil || string(data) != body {
					t.Fatal("configuration was modified")
				}
			}
		})
	}
}

func TestNuGetInheritedDocumentsAndEnvironment(t *testing.T) {
	t.Setenv("DEPTRUST_TEST_NUGET_PUBLIC", "https://api.nuget.org/v3/index.json")
	parent := []byte(`<configuration><packageSources><add key="private" value="https://private.invalid"/></packageSources><packageSourceMapping><packageSource key="private"><package pattern="Company.*"/></packageSource></packageSourceMapping></configuration>`)
	child := []byte(`<configuration><packageSources><clear/><add key="public" value="%DEPTRUST_TEST_NUGET_PUBLIC%"/></packageSources><packageSourceMapping><clear/><packageSource key="public"><package pattern="*"/></packageSource></packageSourceMapping></configuration>`)
	sources, mappings, err := parseNuGetDocuments([][]byte{parent, child})
	if err != nil {
		t.Fatal(err)
	}
	reason, err := nugetSourceRestriction("Company.Secret", sources, mappings)
	if err != nil || reason != "" {
		t.Fatalf("%q %v", reason, err)
	}
}

func TestNuGetMergeMatchesSDK(t *testing.T) {
	if _, err := exec.LookPath("dotnet"); err != nil {
		t.Skip(".NET SDK required")
	}
	dir := t.TempDir()
	env := append(os.Environ(), "DOTNET_CLI_TELEMETRY_OPTOUT=1", "DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1", "DOTNET_NOLOGO=1", "DOTNET_GENERATE_ASPNET_CERTIFICATE=false")
	versionCmd := exec.Command("dotnet", "--version")
	versionCmd.Env = env
	version, err := versionCmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	major, _, ok := strings.Cut(strings.TrimSpace(string(version)), ".")
	if !ok {
		t.Fatal("invalid SDK version")
	}
	project := `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net` + major + `.0</TargetFramework></PropertyGroup><ItemGroup><Reference Include="NuGet.Configuration"><HintPath>$(MSBuildSDKsPath)/../NuGet.Configuration.dll</HintPath></Reference><Reference Include="NuGet.Common"><HintPath>$(MSBuildSDKsPath)/../NuGet.Common.dll</HintPath></Reference></ItemGroup></Project>`
	code, err := os.ReadFile("testdata/nuget-oracle.cs")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"oracle.csproj": []byte(project), "Program.cs": code, "NuGet.Config": []byte(`<configuration><packageSources><clear/></packageSources></configuration>`)} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	build := exec.Command("dotnet", "build", filepath.Join(dir, "oracle.csproj"), "--configfile", filepath.Join(dir, "NuGet.Config"), "-o", filepath.Join(dir, "bin"))
	build.Env = env
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("SDK oracle build: %s %v", out, err)
	}
	parent := `<configuration><packageSources><clear/><add key="public" value="https://api.nuget.org/v3/index.json"/><add key="private" value="https://private.invalid"/></packageSources><packageSourceMapping><clear/><packageSource key="public"><package pattern="*"/></packageSource><packageSource key="private"><package pattern="Company.*"/></packageSource></packageSourceMapping></configuration>`
	for _, child := range []string{
		`<configuration/>`,
		`<configuration><packageSourceMapping><packageSource key="private"><package pattern="Other.*"/></packageSource></packageSourceMapping></configuration>`,
		`<configuration><disabledPackageSources><add key="private" value="true"/></disabledPackageSources></configuration>`,
		`<configuration><packageSourceMapping><clear/><packageSource key="public"><package pattern="*"/></packageSource></packageSourceMapping></configuration>`,
	} {
		parentFile, childFile := filepath.Join(dir, "parent.config"), filepath.Join(dir, "child.config")
		if err := os.WriteFile(parentFile, []byte(parent), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(childFile, []byte(child), 0600); err != nil {
			t.Fatal(err)
		}
		command := exec.Command("dotnet", filepath.Join(dir, "bin/oracle.dll"), childFile, parentFile)
		command.Env = env
		out, err := command.Output()
		if err != nil {
			t.Fatal(err)
		}
		var expected struct {
			IsPublic bool `json:"isPublic"`
		}
		if err := json.Unmarshal(out, &expected); err != nil {
			t.Fatal(err)
		}
		sources, mappings, err := parseNuGetDocuments([][]byte{[]byte(parent), []byte(child)})
		if err != nil {
			t.Fatal(err)
		}
		reason, err := nugetSourceRestriction("Company.Secret", sources, mappings)
		if err != nil {
			t.Fatal(err)
		}
		if (reason == "") != expected.IsPublic {
			t.Fatalf("Go decision disagrees with SDK for %s: %q vs %s", child, reason, out)
		}
	}
}
