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
	if _, err := NormalizeEcosystem("composer"); err == nil {
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
