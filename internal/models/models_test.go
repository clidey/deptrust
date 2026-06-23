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
	if _, err := NormalizeEcosystem("maven"); err == nil {
		t.Fatal("expected unsupported ecosystem error")
	}
}
