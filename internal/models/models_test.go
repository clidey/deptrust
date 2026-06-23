package models

import "testing"

func TestNormalizeEcosystem(t *testing.T) {
	tests := []struct {
		input string
		want  Ecosystem
	}{
		{input: "npm", want: EcosystemNPM},
		{input: "nodejs", want: EcosystemNPM},
		{input: "PyPI", want: EcosystemPyPI},
		{input: "python", want: EcosystemPyPI},
		{input: "cargo", want: EcosystemCargo},
		{input: "rust", want: EcosystemCargo},
		{input: "go", want: EcosystemGo},
		{input: "golang", want: EcosystemGo},
		{input: "rubygems", want: EcosystemRuby},
		{input: "gem", want: EcosystemRuby},
		{input: "nuget", want: EcosystemNuGet},
		{input: "dotnet", want: EcosystemNuGet},
		{input: "maven", want: EcosystemMaven},
		{input: "java", want: EcosystemMaven},
		{input: "composer", want: EcosystemPackagist},
		{input: "php", want: EcosystemPackagist},
		{input: "pub.dev", want: EcosystemPub},
		{input: "dart", want: EcosystemPub},
		{input: "cocoapods", want: EcosystemCocoaPods},
		{input: "pod", want: EcosystemCocoaPods},
		{input: "hex.pm", want: EcosystemHex},
		{input: "elixir", want: EcosystemHex},
		{input: "hackage", want: EcosystemHackage},
		{input: "cabal", want: EcosystemHackage},
		{input: "github-actions", want: EcosystemGitHubActions},
		{input: "gha", want: EcosystemGitHubActions},
	}

	for _, tt := range tests {
		got, err := NormalizeEcosystem(tt.input)
		if err != nil {
			t.Fatalf("NormalizeEcosystem(%q) returned error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("NormalizeEcosystem(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeEcosystemRejectsUnsupported(t *testing.T) {
	if _, err := NormalizeEcosystem("definitely-not-real"); err == nil {
		t.Fatal("expected unsupported ecosystem error")
	}
}

func TestEcosystemProviderNames(t *testing.T) {
	tests := []struct {
		ecosystem Ecosystem
		osv       string
		github    string
	}{
		{ecosystem: EcosystemNPM, osv: "npm", github: "npm"},
		{ecosystem: EcosystemPyPI, osv: "PyPI", github: "pip"},
		{ecosystem: EcosystemCargo, osv: "crates.io", github: "rust"},
		{ecosystem: EcosystemGo, osv: "Go", github: "go"},
		{ecosystem: EcosystemRuby, osv: "RubyGems", github: "rubygems"},
		{ecosystem: EcosystemNuGet, osv: "NuGet", github: "nuget"},
		{ecosystem: EcosystemMaven, osv: "Maven", github: "maven"},
		{ecosystem: EcosystemPackagist, osv: "Packagist", github: "composer"},
		{ecosystem: EcosystemPub, osv: "Pub", github: "pub"},
		{ecosystem: EcosystemCocoaPods, osv: "", github: "swift"},
		{ecosystem: EcosystemHex, osv: "Hex", github: "erlang"},
		{ecosystem: EcosystemHackage, osv: "Hackage", github: ""},
		{ecosystem: EcosystemGitHubActions, osv: "GitHub Actions", github: "actions"},
	}

	for _, tt := range tests {
		if got := tt.ecosystem.OSVEcosystem(); got != tt.osv {
			t.Fatalf("%s.OSVEcosystem() = %q, want %q", tt.ecosystem, got, tt.osv)
		}
		if got := tt.ecosystem.GitHubEcosystem(); got != tt.github {
			t.Fatalf("%s.GitHubEcosystem() = %q, want %q", tt.ecosystem, got, tt.github)
		}
	}
}
