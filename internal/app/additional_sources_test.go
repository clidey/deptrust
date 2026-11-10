package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestGoSavedPrivateSettings(t *testing.T) {
	envFile := filepath.Join(t.TempDir(), "go.env")
	t.Setenv("GOENV", envFile)
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GOTOOLCHAIN", "local")
	if out, err := exec.Command("go", "env", "-w", "GOPRIVATE=corp.example/*").CombinedOutput(); err != nil {
		t.Fatalf("%s %v", out, err)
	}
	query := models.Query{Ecosystem: models.EcosystemGo, Package: "corp.example/secret", Version: "v1.0.0"}
	assertUnavailableSource(t, query)
	t.Setenv("GOPRIVATE", "other.example/*")
	reason, err := sourceRestriction(context.Background(), query)
	if err != nil || reason != "" {
		t.Fatalf("environment did not override saved setting: %q %v", reason, err)
	}
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GOENV", "off")
	reason, err = sourceRestriction(context.Background(), query)
	if err != nil || reason != "" {
		t.Fatalf("GOENV=off: %q %v", reason, err)
	}
}

func assertUnavailableSource(t *testing.T, query models.Query) {
	t.Helper()
	service := App{providers: []vulnerabilityClient{forbiddenPrivateProvider{}}}
	for _, version := range []string{"1.0.0", "latest"} {
		query.Version = version
		got, err := service.CheckPackage(context.Background(), query)
		if err != nil || got.Recommendation != "unknown" || got.SafeToUse || got.ShouldInstall || got.AdvisoryCoverage != "none" {
			t.Fatalf("check: %+v %v", got, err)
		}
	}
	suggestion, err := service.SuggestSafeVersion(context.Background(), query)
	if err != nil || suggestion.Recommendation != "unknown" || suggestion.SuggestedVersion != "" {
		t.Fatalf("suggest: %+v %v", suggestion, err)
	}
	comparison, err := service.CompareVersions(context.Background(), query, "1.0.0", "2.0.0")
	if err != nil || comparison.Recommendation != "unknown" || comparison.ImprovesRisk {
		t.Fatalf("compare: %+v %v", comparison, err)
	}
}

func TestCargoDeclaredSources(t *testing.T) {
	for _, tt := range []struct {
		dep     string
		blocked bool
	}{
		{`{"name":"secret","source":"registry+https://github.com/rust-lang/crates.io-index","registry":null}`, false},
		{`{"name":"secret","source":"registry+https://corp.example/index","registry":"https://corp.example/index"}`, true},
		{`{"name":"secret","source":null,"path":"/tmp/secret"}`, true},
		{`{"name":"secret","source":"git+https://corp.example/secret","registry":null}`, true},
		{`{"name":"other","rename":"secret","source":"registry+https://github.com/rust-lang/crates.io-index"}`, true},
	} {
		reason, err := cargoMetadataRestriction([]byte(`{"packages":[{"name":"app","dependencies":[`+tt.dep+`]}]}`), "secret")
		if err != nil || (reason != "") != tt.blocked {
			t.Fatalf("%s: %q %v", tt.dep, reason, err)
		}
	}
	for _, source := range []string{`null`, `"git+https://corp.example/secret"`, `"registry+https://github.com/rust-lang/crates.io-index"`} {
		reason, err := cargoResolvedRestriction([]byte(`{"packages":[{"name":"secret","source":`+source+`}]}`), "secret")
		if err != nil || (reason == "") != (source == `"registry+https://github.com/rust-lang/crates.io-index"`) {
			t.Fatalf("resolved: %q %v", reason, err)
		}
	}
}

func TestCargoAlternateRegistryOffline(t *testing.T) {
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("Cargo not installed")
	}
	dir := t.TempDir()
	for _, sub := range []string{"src", ".cargo"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"Cargo.toml": "[package]\nname='source-test'\nversion='0.1.0'\nedition='2021'\n[dependencies]\nsecret={version='1',registry='internal'}\n",
		"src/lib.rs": "", ".cargo/config.toml": "[registries.internal]\nindex='sparse+https://corp.invalid/index/'\n",
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	assertUnavailableSource(t, models.Query{Ecosystem: models.EcosystemCargo, Package: "secret", ProjectDir: dir})
	if _, err := os.Stat(filepath.Join(dir, "Cargo.lock")); !os.IsNotExist(err) {
		t.Fatal("metadata unexpectedly wrote lockfile")
	}
}

func TestPoetrySourceConstraints(t *testing.T) {
	for _, tt := range []struct {
		name, config string
		blocked      bool
	}{
		{"explicit custom", "[tool.poetry.dependencies]\nSecret_Package={version='1',source='internal'}\n[[tool.poetry.source]]\nname='internal'\nurl='https://user:secret@corp.invalid/simple'\npriority='explicit'\n", true},
		{"explicit public", "[tool.poetry.dependencies]\nsecret-package={version='1',source='pypi'}\n[[tool.poetry.source]]\nname='internal'\nurl='https://corp.invalid/simple'\npriority='primary'\n", false},
		{"unused explicit", "[tool.poetry.dependencies]\nsecret-package='1'\n[[tool.poetry.source]]\nname='internal'\nurl='https://corp.invalid/simple'\npriority='explicit'\n", false},
		{"unconstrained primary", "[tool.poetry.dependencies]\nsecret-package='1'\n[[tool.poetry.source]]\nname='internal'\nurl='https://corp.invalid/simple'\n", true},
		{"group path", "[tool.poetry.group.dev.dependencies]\nsecret-package={path='../secret'}\n", true},
		{"pep621 public", "[project]\ndependencies=['Secret.Package>=1']\n", false},
		{"pep621 direct", "[project]\ndependencies=['Secret.Package @ https://corp.invalid/package.whl']\n", true},
		{"optional dependency", "[project.optional-dependencies]\nextra=['Secret.Package>=1']\n", false},
		{"dependency group", "[dependency-groups]\ndev=['Secret.Package>=1']\n", false},
		{"not declared", "[project]\ndependencies=['other']\n", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(tt.config), 0600); err != nil {
				t.Fatal(err)
			}
			q := models.Query{Ecosystem: models.EcosystemPyPI, Package: "secret-package", ProjectDir: dir}
			reason, err := sourceRestriction(context.Background(), q)
			if err != nil || (reason != "") != tt.blocked {
				t.Fatalf("%q %v", reason, err)
			}
			if tt.blocked {
				assertUnavailableSource(t, q)
			}
		})
	}
}

func TestCargoLocalPatchIsNotPublic(t *testing.T) {
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("Cargo not installed")
	}
	dir := t.TempDir()
	for _, sub := range []string{"src", "patched", "patched/src"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string]string{
		"Cargo.toml": "[package]\nname='patch-test'\nversion='0.1.0'\nedition='2021'\n[dependencies]\nsecret='1'\n[patch.crates-io]\nsecret={path='patched'}\n",
		"src/lib.rs": "", "patched/src/lib.rs": "",
		"patched/Cargo.toml": "[package]\nname='secret'\nversion='1.0.0'\nedition='2021'\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("cargo", "generate-lockfile", "--offline")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture lock: %s %v", out, err)
	}
	assertUnavailableSource(t, models.Query{Ecosystem: models.EcosystemCargo, Package: "secret", ProjectDir: dir})
}

func TestPoetryInvalidConfigurationStopsLookup(t *testing.T) {
	for _, data := range []string{
		"not valid toml = [",
		"[tool.poetry.dependencies]\nsecret={source='missing',version='1'}\n",
		"[[tool.poetry.source]]\nname='internal'\npriority='unknown'\n",
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := (App{}).CheckPackage(context.Background(), models.Query{Ecosystem: models.EcosystemPyPI, Package: "secret", ProjectDir: dir}); err == nil {
			t.Fatal("expected error before lookup")
		}
	}
}
