package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestNuGetSourceMappings(t *testing.T) {
	for _, tt := range []struct {
		name, mappings, disabled, pkg string
		restricted                    bool
	}{
		{"public wildcard", `<packageSource key="public"><package pattern="*"/></packageSource>`, "", "Example", false},
		{"private prefix wins", `<packageSource key="public"><package pattern="*"/></packageSource><packageSource key="internal"><package pattern="Contoso.*"/></packageSource>`, "", "contoso.Secret", true},
		{"exact beats prefix", `<packageSource key="internal"><package pattern="Contoso.*"/></packageSource><packageSource key="public"><package pattern="Contoso.Open"/></packageSource>`, "", "Contoso.Open", false},
		{"longest prefix", `<packageSource key="internal"><package pattern="Contoso.*"/></packageSource><packageSource key="public"><package pattern="Contoso.Open.*"/></packageSource>`, "", "Contoso.Open.Lib", false},
		{"tied sources", `<packageSource key="public"><package pattern="*"/></packageSource><packageSource key="internal"><package pattern="*"/></packageSource>`, "", "Example", true},
		{"unmapped", `<packageSource key="public"><package pattern="Other"/></packageSource>`, "", "Example", true},
		{"disabled private mapping cannot fall back", `<packageSource key="public"><package pattern="*"/></packageSource><packageSource key="internal"><package pattern="Contoso.*"/></packageSource>`, `<add key="internal" value="true"/>`, "Contoso.Secret", true},
		{"disabled", `<packageSource key="public"><package pattern="*"/></packageSource>`, `<add key="public" value="true"/>`, "Example", true},
		{"no mappings mixed feeds", "", "", "Example", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := filepath.Join(t.TempDir(), "nuget.config")
			data := `<configuration><packageSources><clear/><add key="public" value="https://api.nuget.org/v3/index.json"/><add key="internal" value="https://user:secret@corp.example/feed"/></packageSources><disabledPackageSources>` + tt.disabled + `</disabledPackageSources><packageSourceMapping>` + tt.mappings + `</packageSourceMapping></configuration>`
			if err := os.WriteFile(config, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			q := models.Query{Ecosystem: models.EcosystemNuGet, Package: tt.pkg, Version: "1.0.0", NuGetConfig: config}
			reason, err := sourceRestriction(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			if (reason != "") != tt.restricted {
				t.Fatalf("reason=%q", reason)
			}
			if tt.restricted {
				service := App{providers: []vulnerabilityClient{forbiddenPrivateProvider{}}}
				got, err := service.CheckPackage(context.Background(), q)
				if err != nil || got.Recommendation != "unknown" || got.SafeToUse {
					t.Fatalf("%+v %v", got, err)
				}
				suggestion, err := service.SuggestSafeVersion(context.Background(), q)
				if err != nil || suggestion.Recommendation != "unknown" || suggestion.SuggestedVersion != "" {
					t.Fatalf("%+v %v", suggestion, err)
				}
				comparison, err := service.CompareVersions(context.Background(), q, "1.0.0", "2.0.0")
				if err != nil || comparison.Recommendation != "unknown" || comparison.ImprovesRisk {
					t.Fatalf("%+v %v", comparison, err)
				}
			}
		})
	}
}

func TestSelectedSourceErrorsStopLookup(t *testing.T) {
	for _, q := range []models.Query{
		{Ecosystem: models.EcosystemNuGet, Package: "Secret", NuGetConfig: "relative.config"},
		{Ecosystem: models.EcosystemNuGet, Package: "Secret", NuGetConfig: filepath.Join(t.TempDir(), "missing.config")},
		{Ecosystem: models.EcosystemNPM, Package: "@corp/secret", ProjectDir: "relative"},
	} {
		if _, err := (App{}).CheckPackage(context.Background(), q); err == nil {
			t.Fatal("expected error before lookup")
		}
	}
}

func TestNPMEffectiveRegistry(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm required for real config reader test")
	}
	// Exercise the real npm configuration reader, isolated from user configuration.
	dir := t.TempDir()
	t.Setenv("NPM_CONFIG_USERCONFIG", filepath.Join(dir, "user.npmrc"))
	t.Setenv("NPM_CONFIG_GLOBALCONFIG", filepath.Join(dir, "global.npmrc"))
	t.Setenv("npm_config_registry", "https://registry.npmjs.org/")
	if err := os.WriteFile(filepath.Join(dir, ".npmrc"), []byte("@corp:registry=https://corp.example/npm/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	q := models.Query{Ecosystem: models.EcosystemNPM, Package: "@corp/secret", Version: "1.0.0", ProjectDir: dir}
	service := App{providers: []vulnerabilityClient{forbiddenPrivateProvider{}}}
	got, err := service.CheckPackage(context.Background(), q)
	if err != nil || got.Recommendation != "unknown" {
		t.Fatalf("%+v %v", got, err)
	}
	suggestion, err := service.SuggestSafeVersion(context.Background(), q)
	if err != nil || suggestion.Recommendation != "unknown" {
		t.Fatalf("%+v %v", suggestion, err)
	}
	comparison, err := service.CompareVersions(context.Background(), q, "1.0.0", "2.0.0")
	if err != nil || comparison.Recommendation != "unknown" {
		t.Fatalf("%+v %v", comparison, err)
	}
	q.Package = "@public/example"
	reason, err := sourceRestriction(context.Background(), q)
	if err != nil || reason != "" {
		t.Fatalf("%q %v", reason, err)
	}
}

func TestNuGetUnsupportedDirectivesStopLookup(t *testing.T) {
	for _, body := range []string{
		`<packageSources/><packageSources/>`,
		`<packageSources>`,
	} {
		file := filepath.Join(t.TempDir(), "nuget.config")
		if err := os.WriteFile(file, []byte(`<configuration>`+body+`</configuration>`), 0600); err != nil {
			t.Fatal(err)
		}
		q := models.Query{Ecosystem: models.EcosystemNuGet, Package: "Secret", NuGetConfig: file}
		if _, err := (App{}).CheckPackage(context.Background(), q); err == nil {
			t.Fatalf("expected config error for %s", body)
		}
	}
}

func TestNuGetOrderedCollectionOperations(t *testing.T) {
	for _, tt := range []struct {
		section, disabled string
		blocked           bool
	}{
		{`<add key="private" value="https://private.invalid"/><clear/><add key="public" value="https://api.nuget.org/v3/index.json"/>`, "", false},
		{`<add key="private" value="https://private.invalid"/><remove key="private"/><add key="public" value="https://api.nuget.org/v3/index.json"/>`, "", false},
		{`<add key="public" value="https://api.nuget.org/v3/index.json"/>`, `<add key="public" value="true"/><clear/>`, false},
		{`<add key="public" value="https://api.nuget.org/v3/index.json"/><clear/>`, "", true},
	} {
		file := filepath.Join(t.TempDir(), "nuget.config")
		body := `<configuration><packageSources>` + tt.section + `</packageSources><disabledPackageSources>` + tt.disabled + `</disabledPackageSources></configuration>`
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		q := models.Query{Ecosystem: models.EcosystemNuGet, Package: "Secret", NuGetConfig: file}
		reason, err := sourceRestriction(context.Background(), q)
		if err != nil || (reason != "") != tt.blocked {
			t.Fatalf("%q %v", reason, err)
		}
		if tt.blocked {
			assertUnavailableSource(t, q)
		}
	}
}
